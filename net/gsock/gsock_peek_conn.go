package gsock

import (
	"bufio"
	"net"
	"time"
)

// peekConn is a net.Conn wrapper that provides a peek
type peekConn struct {
	net.Conn
	reader *bufio.Reader
}

// newPeekConn creates a new peekConn
func newPeekConn(conn net.Conn) *peekConn {
	return &peekConn{
		Conn:   conn,
		reader: bufio.NewReader(conn),
	}
}

// Read reads data from the connection.
func (c *peekConn) Read(b []byte) (int, error) {
	return c.reader.Read(b)
}

// Peek peeks at the next n bytes of the connection.
func (c *peekConn) Peek(n int) ([]byte, error) {
	return c.reader.Peek(n)
}

func (c *peekConn) SetDeadline(t time.Time) error {
	return c.Conn.SetReadDeadline(t)
}

// SetReadDeadline sets the read deadline associated with the connection.
func (c *peekConn) SetReadDeadline(t time.Time) error {
	return c.Conn.SetReadDeadline(t)
}

func (c *peekConn) SetWriteDeadline(t time.Time) error {
	return c.Conn.SetWriteDeadline(t)
}
