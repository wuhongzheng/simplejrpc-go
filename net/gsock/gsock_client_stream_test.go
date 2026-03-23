package gsock

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

const TestSocketPath = "/tmp/simplejrpc_stream_test.sock"

type requestExTestHandler struct{}

func (h *requestExTestHandler) Hello(req *Request) (any, error) {
	return "call end!", nil
}

func (h *requestExTestHandler) StreamHello(req *Request) (any, error) {
	var args struct {
		Prefix string `json:"prefix"`
		Count  int    `json:"count"`
	}
	_ = req.Bind(&args)
	if args.Prefix == "" {
		args.Prefix = "item"
	}
	if args.Count <= 0 {
		args.Count = 3
	}

	return Stream(func(ctx context.Context, send func(event string, data any) error) error {
		for i := 1; i <= args.Count; i++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if err := send(StreamEventContent, fmt.Sprintf("%s-%d", args.Prefix, i)); err != nil {
				return err
			}
			time.Sleep(10 * time.Millisecond)
		}
		return nil
	}), nil
}

func (h *requestExTestHandler) TimeoutStream(req *Request) (any, error) {
	return Stream(func(ctx context.Context, send func(event string, data any) error) error {
		<-ctx.Done()
		return ctx.Err()
	}), nil
}

// startRequestExTestServer is removed as per instructions, assuming an external server is running.
// The TestRequestExFull function will check for the server's presence.
func TestRequestExFull(t *testing.T) {
	// Check if server is running
	if _, err := os.Stat(TestSocketPath); err != nil {
		t.Skipf("Server not running at %s, skipping client tests", TestSocketPath)
	}

	client := NewRpcSimpleClient(TestSocketPath)

	// 1. Test Ordinary Unary
	t.Run("Unary", func(t *testing.T) {
		params := map[string]any{
			"name": "world",
		}
		err := client.RequestEx(context.Background(), "hello", params, func(frame StreamFrame) error {
			b, _ := json.Marshal(frame)
			fmt.Printf("Unary received: %s\n", string(b))
			if frame.Data != "hello, world!" {
				t.Errorf("Unexpected Unary response: %v", frame.Data)
			}
			return nil
		}, UnaryHeader())
		if err != nil {
			t.Fatalf("Unary RequestEx failed: %v", err)
		}
	})

	// 2. Test Stream
	t.Run("Stream", func(t *testing.T) {
		var frames []StreamFrame
		err := client.RequestEx(context.Background(), "streamHello", map[string]any{
			"prefix": "test",
			"count":  5,
		}, func(frame StreamFrame) error {
			frames = append(frames, frame)
			b, _ := json.Marshal(frame)
			fmt.Printf("[Client] Stream received: %s\n", string(b))
			return nil
		}, StreamHeader())

		if err != nil {
			t.Fatalf("Stream RequestEx failed: %v", err)
		}
		contentCount := 0
		for _, f := range frames {
			if f.Event == StreamEventContent {
				contentCount++
			}
		}
		if contentCount != 5 {
			t.Fatalf("Expected 5 content frames, got %d", contentCount)
		}
		if !frames[len(frames)-1].Done {
			t.Fatalf("Expected last frame to be Done")
		}
	})

	// 3. Test Timeout Scenario
	t.Run("Timeout", func(t *testing.T) {
		// Test server-side timeout (triggered by our specific mock logic in server_stream_test.go)
		err := client.RequestEx(context.Background(), "timeoutStream", nil, func(frame StreamFrame) error {
			// Catching and printing the error frame sent by the server
			b, _ := json.Marshal(frame)
			fmt.Printf("[Client] Server Error Frame Received: %s\n", string(b))
			return nil
		}, StreamHeader())

		if err == nil {
			t.Fatal("Expected error from server, got nil")
		}
		// Based on our server-side handler, it should return "stream request timeout" (Code 408)
		if !strings.Contains(err.Error(), "stream request timeout") {
			t.Fatalf("Expected 'stream request timeout' error, got: %v", err)
		}
		fmt.Printf("[Client] Verified: Successfully captured server-side timeout error: %v\n", err)
	})

	// 4. Multi-goroutine Concurrent Testing (Cross-talk / Interleaving check)
	t.Run("Concurrency", func(t *testing.T) {
		const goroutines = 3     // Simplified to 3 as requested for better visibility
		const countPerStream = 5 // items per stream
		var wg sync.WaitGroup
		wg.Add(goroutines)

		for i := 0; i < goroutines; i++ {
			go func(idx int) {
				defer wg.Done()
				prefix := fmt.Sprintf("g%d", idx)
				var lastSeq int = -1
				var expectedStreamID string
				contentReceived := 0

				// Each goroutine uses its own client (which dials its own connection)
				c := NewRpcSimpleClient(TestSocketPath)
				err := c.RequestEx(context.Background(), "streamHello", map[string]any{
					"prefix": prefix,
					"count":  countPerStream,
				}, func(frame StreamFrame) error {
					if frame.Event == StreamEventStart {
						expectedStreamID = frame.StreamID
						fmt.Printf(">>> [Goroutine %d] Stream started | ID: %s\n", idx, expectedStreamID)
					}

					if frame.Event == StreamEventContent {
						contentReceived++
						data := frame.Data.(string)

						// 1. Verify StreamID consistency (to prevent cross-stream frame leakage)
						if expectedStreamID != "" && frame.StreamID != expectedStreamID {
							t.Errorf("[CRITICAL] StreamID mismatch for goroutine %d! Expected %s, got %s", idx, expectedStreamID, frame.StreamID)
						}

						// 2. Verify Data Prefix (to detect logic interleaving)
						// If we receive "gx-idx" where x != idx, it's an interleaved response
						if !strings.HasPrefix(data, prefix+"-") {
							t.Errorf("[CRITICAL] Interleaving detected for goroutine %d! Expected prefix '%s-', got '%s'", idx, prefix, data)
							return fmt.Errorf("interleaving detected: %s contains data from other stream", data)
						}

						// 3. Verify Sequence Ordering
						if frame.Seq <= lastSeq {
							t.Errorf("[CRITICAL] Out-of-order frame for goroutine %d! Seq %d <= last %d", idx, frame.Seq, lastSeq)
						}
						lastSeq = frame.Seq

						// Log the verification success for each frame to make it visible to the user
						fmt.Printf("    [Goroutine %d] Verification PASS -> data=%s seq=%d [ID: %s]\n", idx, data, frame.Seq, frame.StreamID)
					}
					return nil
				}, StreamHeader())

				if err != nil {
					t.Errorf("Goroutine %d failed: %v", idx, err)
				}
				if contentReceived != countPerStream {
					t.Errorf("Goroutine %d expected %d items, got %d", idx, countPerStream, contentReceived)
				}
			}(i)
		}
		wg.Wait()
	})
}
