package gsock

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync"
)

const (
	Endpoint = "endpoint"
	Close    = "close"
)

type StreamResponder interface {
	StreamID() string
	Used() bool
	Ended() bool

	Write(data any) error
	WriteString(data string) error

	// Start stream start
	Start(meta map[string]any) error
	// Send stream data
	Send(event string, data any) error
	// End stream endz
	End(meta map[string]any) error
	// Fail stream fail
	Fail(code int, message string) error
}

// rpcSteamResponder is a gsock stream responder
type rpcSteamResponder struct {
	ctx      context.Context
	conn     net.Conn
	codec    FrameCodec
	reqID    uint64
	method   string
	streamID string

	mu    sync.Mutex
	seq   int
	used  bool
	ended bool
}

// newStreamResponder creates a new stream responder
func newStreamResponder(
	ctx context.Context,
	conn net.Conn,
	codec FrameCodec,
	reqID uint64,
	method string,
	streamID string) StreamResponder {
	if streamID == "" {
		streamID = nextStreamID()
	}
	return &rpcSteamResponder{
		ctx:      ctx,
		conn:     conn,
		codec:    codec,
		reqID:    reqID,
		method:   method,
		streamID: streamID,
	}
}

func (r *rpcSteamResponder) StreamID() string {
	return r.streamID
}

func (r *rpcSteamResponder) Used() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.used
}

func (r *rpcSteamResponder) Ended() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ended
}

func (r *rpcSteamResponder) Write(data any) error {
	if err := r.ensureStared(); err != nil {
		return err
	}
	return r.Send(StreamEventContent, data)
}

func (r *rpcSteamResponder) WriteString(data string) error {
	return r.Write(data)
}

func (r *rpcSteamResponder) ensureStared() error {
	r.mu.Lock()
	used := r.used
	ended := r.ended
	r.mu.Unlock()

	if used || ended {
		return nil
	}

	return r.Start(map[string]any{
		Endpoint: r.method,
		Close:    0,
	})
}

func (r *rpcSteamResponder) Start(meta map[string]any) error {
	return r.writeFrame(StreamFrame{
		Code:     http.StatusOK,
		Msg:      http.StatusText(http.StatusOK),
		Stream:   true,
		Seq:      r.seq,
		Meta:     meta,
		StreamID: r.streamID,
		Event:    StreamEventStart,
		Done:     false,
	})
}

func (r *rpcSteamResponder) Send(event string, data any) error {
	if event == "" {
		event = StreamEventContent
	}
	return r.writeFrame(StreamFrame{
		Code:     http.StatusOK,
		Msg:      http.StatusText(http.StatusOK),
		Stream:   true,
		Seq:      r.seq,
		Data:     data,
		StreamID: r.streamID,
		Event:    event,
		Done:     false,
	})
}

func (r *rpcSteamResponder) End(meta map[string]any) error {
	return r.writeFrame(StreamFrame{
		Code:     http.StatusOK,
		Msg:      http.StatusText(http.StatusOK),
		Stream:   true,
		Seq:      r.seq,
		Meta:     meta,
		StreamID: r.streamID,
		Event:    StreamEventEnd,
		Done:     true,
	})
}

func (r *rpcSteamResponder) Fail(code int, message string) error {
	if code == 0 {
		code = http.StatusInternalServerError
	}
	if message == "" {
		message = http.StatusText(code)
	}
	return r.writeFrame(StreamFrame{
		Code:     code,
		Msg:      message,
		Data:     nil,
		StreamID: r.streamID,
		Event:    StreamEventError,
		Done:     true,
		Meta: map[string]any{
			Endpoint: r.method,
			Close:    1,
		},
	})
}

func (r *rpcSteamResponder) writeFrame(frame StreamFrame) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ended {
		return nil
	}

	r.used = true
	r.seq++
	frame.Seq = r.seq

	if frame.StreamID == "" {
		frame.StreamID = r.streamID
	}

	body, err := json.Marshal(frame)
	if err != nil {
		return err
	}

	if err := r.codec.WriteFrame(r.conn, &Frame{
		Header: Header{
			Mode:   CallModeStream,
			Length: uint32(len(body)),
		},
		Payload: body,
	}); err != nil {
		return err
	}
	if frame.Done {
		r.ended = true
	}
	return nil
}
