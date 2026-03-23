package gsock

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
)

const (
	ProtocolVersion uint8 = 1
)

type CallMode uint8

const (
	CallModeUnary  CallMode = 1
	CallModeStream CallMode = 2
)

const (
	StreamEventStart   = "start"
	StreamEventContent = "content"
	StreamEventError   = "error"
	StreamEventEnd     = "end"
)

// Header defines the header of a gsock message
type Header struct {
	Version uint8
	Mode    CallMode
	Flags   uint16
	Length  uint32
}

// UnaryHeader returns a default protocol header configured for unary requests.
func UnaryHeader() Header {
	return Header{
		Version: ProtocolVersion,
		Mode:    CallModeUnary,
	}
}

// StreamHeader returns a default protocol header configured for streaming requests.
func StreamHeader() Header {
	return Header{
		Version: ProtocolVersion,
		Mode:    CallModeStream,
	}
}

// Frame defines a gsock message
type Frame struct {
	Header  Header
	Payload []byte
}

// RPCRequest defines a JSON-RPC request
type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RPCError defines a JSON-RPC error response
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// StreamFrame is the unified response envelope used by RequestEx.
//
// Unary requests produce a single frame with Stream=false and Done=true.
// Stream requests produce one or more frames with Stream=true, ending with Done=true.
type StreamFrame struct {
	Code     int            `json:"code"`
	Msg      string         `json:"msg"`
	Stream   bool           `json:"stream"`
	Data     any            `json:"data"`
	StreamID string         `json:"stream_id,omitempty"`
	Event    string         `json:"event,omitempty"`
	Seq      int            `json:"seq,omitempty"`
	Done     bool           `json:"done"`
	Meta     map[string]any `json:"meta,omitempty"`
}

// DefaultStreamHandleTimeout defines the default server-side timeout for a single stream request.
const DefaultStreamHandleTimeout = 60

// StreamResult defines a result for a streaming request.
type StreamResult struct {
	Producer func(ctx context.Context, send func(event string, data any) error) error
}

// Stream defines a streaming request.
func Stream(producer func(ctx context.Context, send func(event string, data any) error) error) *StreamResult {
	return &StreamResult{Producer: producer}
}

var (
	globalRequestID uint64
	globalStreamID  uint64
)

// nextRequestID returns the next request ID
func nextRequestID() uint64 {
	return atomic.AddUint64(&globalRequestID, 1)
}

// nextStreamID returns the next stream ID
func nextStreamID() string {
	v := atomic.AddUint64(&globalStreamID, 1)
	return fmt.Sprintf("stream-%d", v)
}
