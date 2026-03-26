package gsock

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// FrameStreamClient implements frame-based streaming request handling.
type FrameStreamClient struct {
	conn  net.Conn
	codec FrameCodec
}

// NewFrameClient creates a frame stream client with the provided connection and frameCodec.
// If frameCodec is nil, LengthFrameCodec is used by default.
func NewFrameClient(conn net.Conn, codec FrameCodec) *FrameStreamClient {
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

// RequestWithFrame sends a stream request over the frame protocol
// and forwards each decoded StreamFrame to onResponse until the stream completes.
func (c *FrameStreamClient) RequestWithFrame(
	ctx context.Context,
	method string,
	params any,
	onResponse ResponseHandler,
	header *Header,
) error {
	if header == nil {
		return fmt.Errorf("stream error: code=%d msg=%s", http.StatusBadRequest, "missing request header")
	}
	if header.Mode != CallModeStream {
		return fmt.Errorf("stream error: code=%d msg=%s", http.StatusBadRequest, "invalid request mode")
	}

	req, err := buildFrameRPCRequest(method, params)
	if err != nil {
		return err
	}

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	// write frame
	if err := c.codec.WriteFrame(c.conn, &Frame{
		Header:  *header,
		Payload: body,
	}); err != nil {
		return err
	}

	// undefine read deadline
	if dl, ok := ctx.Deadline(); ok {
		if err := c.conn.SetReadDeadline(dl); err != nil {
			return err
		}
	}

	// reset read deadline
	defer func(conn net.Conn, t time.Time) {
		err := conn.SetReadDeadline(t)
		if err != nil {
		}
	}(c.conn, time.Time{})

	for {
		// check context status
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		frame, err := c.codec.ReadFrame(c.conn)
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			return err
		}

		var streamFrame StreamFrame
		if err := json.Unmarshal(frame.Payload, &streamFrame); err != nil {
			return err
		}

		// Process the response of the data through the registered function
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
		JSONRPC: JSONRPCVersion,
		ID:      nextRequestID(),
		Method:  method,
		Params:  rawParams,
	}, nil
}
