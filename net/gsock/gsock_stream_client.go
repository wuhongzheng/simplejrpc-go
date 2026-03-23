package gsock

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
)

// FrameStreamClient implements frame-based streaming request handling.
type FrameStreamClient struct {
	conn  net.Conn
	codec FrameCodec
}

// NewFrameStreamClient creates a frame stream client with the provided connection and codec.
// If codec is nil, LengthFrameCodec is used by default.
func NewFrameStreamClient(conn net.Conn, codec FrameCodec) *FrameStreamClient {
	if codec == nil {
		codec = &LengthFrameCodec{}
	}
	return &FrameStreamClient{
		conn:  conn,
		codec: codec,
	}
}

// Request is unsupported for FrameStreamClient.
// Use RequestWithFrame for frame-mode request handling.
func (c *FrameStreamClient) Request(
	ctx context.Context,
	method string,
	params, result any,
) error {
	_ = ctx
	_ = method
	_ = params
	_ = result
	return fmt.Errorf("frame stream client only supports RequestExStream")
}

// RequestWithFrame sends a request over the frame protocol
// and forwards each decoded StreamFrame to onResponse until the stream completes.
func (c *FrameStreamClient) RequestWithFrame(
	ctx context.Context,
	method string,
	params any,
	onResponse ResponseHandler,
	header *Header,
) error {
	req, err := buildFrameRPCRequest(method, params)
	if err != nil {
		return err
	}

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	h := StreamHeader()
	if header != nil {
		h = *header
		if h.Version == 0 {
			h.Version = ProtocolVersion
		}
		if h.Mode == 0 {
			h.Mode = CallModeStream
		}
	}

	if err := c.codec.WriteFrame(c.conn, &Frame{
		Header:  h,
		Payload: body,
	}); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		frame, err := c.codec.ReadFrame(c.conn)
		if err != nil {
			return err
		}

		var streamFrame StreamFrame
		if err := json.Unmarshal(frame.Payload, &streamFrame); err != nil {
			return err
		}

		if onResponse != nil {
			if err := onResponse(streamFrame); err != nil {
				return err
			}
		}

		if streamFrame.Done {
			if streamFrame.Code >= http.StatusBadRequest {
				return fmt.Errorf("stream error: code=%d msg=%s", streamFrame.Code, streamFrame.Msg)
			}
			return nil
		}
	}
}

// buildFrameRPCRequest builds a JSON-RPC request payload for frame-based transport.
func buildFrameRPCRequest(method string, params any) (*RPCRequest, error) {
	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		rawParams = b
	}

	return &RPCRequest{
		JSONRPC: "2.0",
		ID:      nextRequestID(),
		Method:  method,
		Params:  rawParams,
	}, nil
}
