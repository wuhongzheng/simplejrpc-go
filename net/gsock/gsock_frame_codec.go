package gsock

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
)

// FrameCodec defines the interface for encoding and decoding frames.
type FrameCodec interface {
	// ReadFrame reads a frame from the connection.
	ReadFrame(conn net.Conn) (*Frame, error)
	// WriteFrame writes a frame to the connection.
	WriteFrame(conn net.Conn, frame *Frame) error
}

type LengthFrameCodec struct {
}

// ReadFrame impl reads a frame from the connection.
func (c *LengthFrameCodec) ReadFrame(conn net.Conn) (*Frame, error) {
	var rawHeader [8]byte
	if _, err := io.ReadFull(conn, rawHeader[:]); err != nil {
		return nil, err
	}

	header := Header{
		Version: rawHeader[0],
		Mode:    CallMode(rawHeader[1]),
		Flags:   binary.BigEndian.Uint16(rawHeader[2:4]),
		Length:  binary.BigEndian.Uint32(rawHeader[4:8]),
	}

	if header.Length == 0 {
		return nil, errors.New("empty payload")
	}

	body := make([]byte, header.Length)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, err
	}

	return &Frame{
		Header:  header,
		Payload: body,
	}, nil
}

// WriteFrame impl writes a frame to the connection.
func (c *LengthFrameCodec) WriteFrame(conn net.Conn, frame *Frame) error {
	frame.Header.Length = uint32(len(frame.Payload))

	var rawHeader [8]byte
	rawHeader[0] = frame.Header.Version
	rawHeader[1] = byte(frame.Header.Mode)
	binary.BigEndian.PutUint16(rawHeader[2:4], frame.Header.Flags)
	binary.BigEndian.PutUint32(rawHeader[4:8], frame.Header.Length)

	if err := writeFull(conn, rawHeader[:]); err != nil {
		return err
	}
	return writeFull(conn, frame.Payload)
}

func writeFull(conn net.Conn, b []byte) error {
	for len(b) > 0 {
		n, err := conn.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}
