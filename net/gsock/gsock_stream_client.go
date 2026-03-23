package gsock

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/sourcegraph/jsonrpc2"
)

type FrameStreamClient struct {
	conn  net.Conn
	codec FrameCodec
}

func NewFrameStreamClient(conn net.Conn, codec FrameCodec) *FrameStreamClient {
	if codec == nil {
		codec = &LengthFrameCodec{}
	}
	return &FrameStreamClient{
		conn:  conn,
		codec: codec,
	}
}

func (c *FrameStreamClient) Request(
	ctx context.Context,
	method string,
	params, result any,
	opts ...jsonrpc2.CallOption,
) error {
	_ = ctx
	_ = method
	_ = params
	_ = result
	_ = opts
	return fmt.Errorf("frame stream client only supports RequestStream")
}

func (c *FrameStreamClient) RequestStream(
	ctx context.Context,
	method string,
	params any,
	onStream StreamHandler,
	_ ...jsonrpc2.CallOption,
) error {

	req, err := buildFrameRPCRequest(method, params)
	if err != nil {
		return err
	}

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	if err := c.codec.WriteFrame(c.conn, &Frame{
		Header: Header{
			Version: ProtocolVersion,
			Mode:    CallModeStream,
		},
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

		if onStream != nil {
			if err := onStream(streamFrame); err != nil {
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
