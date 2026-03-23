package gsock

import (
	"context"
	"encoding/json"
)

type RawRequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      uint64           `json:"id"`
	Method  string           `json:"method"`
	Params  *json.RawMessage `json:"params,omitempty"`
}

// RequestOptFunc defines functions for configuring Request objects
type RequestOptFunc func(*Request)

// Request wraps a JSON-RPC 2.0 request with additional context and functionality
type Request struct {
	ctx      context.Context // Context for cancellation/timeout
	raw      *RawRequest
	mode     CallMode
	streamID string
	stream   StreamResponder
}

// RawRequest returns the underlying JSON-RPC 2.0 request object
// Useful for accessing low-level request details when needed
func (r *Request) RawRequest() *RawRequest {
	return r.raw
}

// Method returns the RPC method name being called
// This is a convenience method that delegates to the underlying request
func (r *Request) Method() string {
	if r.raw == nil {
		return ""
	}
	return r.raw.Method
}

// Context returns the request's context
// The context carries deadlines, cancellation signals, and other request-scoped values
func (r *Request) Context() context.Context {
	return r.ctx
}

func (r *Request) Mode() CallMode {
	if r.mode == 0 {
		return CallModeUnary
	}
	return r.mode
}

func (r *Request) IsStream() bool {
	return r.Mode() == CallModeStream
}

func (r *Request) StreamID() string {
	return r.streamID
}

func (r *Request) Stream() StreamResponder {
	return r.stream
}

func (r *Request) StreamResponder() StreamResponder {
	return r.stream
}

func (r *Request) Bind(v any) error {
	if r.raw == nil || r.raw.Params == nil || len(*r.raw.Params) == 0 {
		return nil
	}
	return json.Unmarshal(*r.raw.Params, v)
}

// WithRequestCtxOption creates a RequestOptFunc that sets the request context
// This is typically used to propagate cancellation signals and deadlines
func WithRequestCtxOption(ctx context.Context) RequestOptFunc {
	return func(r *Request) {
		r.ctx = ctx
	}
}

// WithRequestReqOption creates a RequestOptFunc that sets the underlying JSON-RPC request
// This is used when wrapping an existing JSON-RPC request
func WithRequestReqOption(raw *RawRequest) RequestOptFunc {
	return func(r *Request) {
		r.raw = raw
	}
}

func WithRequestModeOption(mode CallMode) RequestOptFunc {
	return func(r *Request) {
		r.mode = mode
	}
}

func WithRequestStreamIDOption(streamID string) RequestOptFunc {
	return func(r *Request) {
		r.streamID = streamID
	}
}

func WithRequestStreamOption(stream StreamResponder) RequestOptFunc {
	return func(r *Request) {
		r.stream = stream
	}
}

// MakeRequest constructs a new Request instance with the provided options
// This follows the functional options pattern for flexible request creation
//
// Example:
//
//	req := MakeRequest(
//	    WithRequestCtxOption(ctx),
//	    WithRequestReqOption(jsonReq),
//	)
func MakeRequest(opts ...RequestOptFunc) *Request {
	r := &Request{
		ctx:  context.Background(), // Default context
		mode: CallModeUnary,
	}

	// Apply all configuration options
	for _, opt := range opts {
		opt(r)
	}
	return r
}
