package simplejrpc

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DemonZack/simplejrpc-go/net/gsock"
)

const TestSocketPath = "/tmp/simplejrpc_stream_test.sock"

type streamTestHandler struct{}

func (h *streamTestHandler) Hello(req *gsock.Request) (any, error) {
	var args struct {
		Name string `json:"name"`
	}
	_ = req.Bind(&args)
	if args.Name == "" {
		args.Name = "world"
	}
	return fmt.Sprintf("hello, %s!", args.Name), nil
}

func (h *streamTestHandler) StreamHello(req *gsock.Request) (any, error) {
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

	return gsock.Stream(func(ctx context.Context, send func(event string, data any) error) error {
		for i := 1; i <= args.Count; i++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if err := send(gsock.StreamEventContent, fmt.Sprintf("%s-%d", args.Prefix, i)); err != nil {
				return err
			}
			time.Sleep(10 * time.Millisecond)
		}
		return nil
	}), nil
}

func (h *streamTestHandler) TimeoutStream(req *gsock.Request) (any, error) {
	return gsock.Stream(func(ctx context.Context, send func(event string, data any) error) error {
		// Use a local timeout within the producer to simulate a server-side timeout faster than 60s
		// Returning context.DeadlineExceeded here will cause gsock to send "stream request timeout"
		timeout := 2 * time.Second
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(timeout):
			return context.DeadlineExceeded
		}
	}), nil
}

func startOuterStreamTestServer(t *testing.T) {
	t.Helper()

	_ = os.Remove(TestSocketPath)

	ds := NewDefaultServer(
		gsock.WithJsonRpcSimpleServiceHandler(gsock.NewJsonRpcSimpleServiceHandler()),
	)

	hand := &streamTestHandler{}
	ds.RegisterHandle("hello", hand.Hello)
	ds.RegisterHandle("streamHello", hand.StreamHello)
	ds.RegisterHandle("timeoutStream", hand.TimeoutStream)

	fmt.Printf("Starting server on %s...\n", TestSocketPath)
	go func() {
		if err := ds.StartServer(TestSocketPath); err != nil {
			fmt.Printf("Server exited with error: %v\n", err)
		}
	}()

	// Wait for socket to be ready
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(TestSocketPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Server failed to start and create socket")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestServerKeepRunning starts the server and keeps it running for manual or cross-package testing
func TestServerKeepRunning(t *testing.T) {
	startOuterStreamTestServer(t)
	fmt.Println("Server is running. Waiting for client connections...")
	// Keep alive for a long time or until interrupted
	// In actual unit tests, we'd use a signal or a waitgroup,
	// but here we follow the user's request to "wait for client connections".
	time.Sleep(10 * time.Minute)
	fmt.Println("Server timeout after 10 minutes")
}
