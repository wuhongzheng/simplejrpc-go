package gsock

import (
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

func UnaryHeader() Header {
	return Header{
		Version: ProtocolVersion,
		Mode:    CallModeUnary,
	}
}

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

type StreamFrame struct {
	Code     int            `json:"code"`
	Msg      string         `json:"msg"`
	Stream   bool           `json:"stream"`
	Data     any            `json:"data"`
	StreamID string         `json:"stream_id,omitempty"`
	Event    string         `json:"event,omitempty"`
	Seq      int            `json:"seq,omitempty"`
	Done     bool           `json:"done"`
	Meta     map[string]any `json:"meta"`
}

// StreamResult defines a result for a streaming request
type StreamResult struct {
	Producer func(send func(event string, data any) error) error
}

// Stream defines a streaming request
func Stream(producer func(send func(event string, data any) error) error) *StreamResult {
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
