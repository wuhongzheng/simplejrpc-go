package gsock

import (
	"context"
	"fmt"
	"net"

	"github.com/sourcegraph/jsonrpc2"
)

// RPCClient provides JSON-RPC 2.0 client functionality over Unix domain sockets.
// It manages connection lifecycle and request/response handling through a configurable adapter.
//
// Fields:
//   - sockPath:  Filesystem path to the Unix domain socket (e.g., "/tmp/rpc.sock")
//   - adapter:   Protocol adapter implementing the ClientAdapter interface (defaults to JSON-RPC)
//   - idCounter: Atomic counter for generating unique request IDs
//   - keepLive:  Flag controlling whether connections should be kept alive after requests
type rpcClient struct {
	sockPath    string        // Path to the Unix domain socket
	adapter     ClientAdapter // Protocol adapter (defaults to JSON-RPC)
	streamCodec FrameCodec
	idCounter   int64 // Atomic counter for generating request IDs
	keepLive    bool  // Connection persistence flag
}

// NewRpcKeepLivClient creates a new RPC client with connection persistence disabled.
// Connections will remain open after requests (caller must manage cleanup).
//
// Parameters:
//   - socketPath: Absolute filesystem path to the Unix domain socket
//
// Returns:
//   - *rpcClient: Initialized client instance ready for RPC calls
//
// Note:
//
//	Uses JsonRpcSimpleClient as the default adapter
func NewRpcSimpleClient(socketPath string) *rpcClient {
	return &rpcClient{
		sockPath:    socketPath,
		adapter:     &JsonRpcSimpleClient{},
		streamCodec: &LengthFrameCodec{},
	}
}

// NewRpcSimpleClient creates a new RPC client with connection persistence enabled.
// The client will automatically close connections after each request.
//
// Parameters:
//   - socketPath: Absolute filesystem path to the Unix domain socket
//
// Returns:
//   - *rpcClient: Initialized client instance with keepalive disabled
//
// Note:
//
//	Uses JsonRpcSimpleClient as the default adapter
func NewRpcKeepLiveClient(socketPath string) *rpcClient {
	return &rpcClient{
		sockPath:    socketPath,
		adapter:     &JsonRpcSimpleClient{},
		streamCodec: &LengthFrameCodec{},
		keepLive:    true,
	}
}

// Request executes a JSON-RPC 2.0 method call and handles response decoding.
// Automatically manages connection establishment, request ID generation, and error handling.
//
// Parameters:
//   - ctx:       Context for cancellation and timeout control
//   - method:    RPC method name to invoke
//   - params:    Input parameters (will be JSON-serialized)
//   - result:    Pointer to structure for response deserialization
//   - opts:      Optional JSON-RPC 2.0 call configurations
//
// Returns:
//   - error:     nil on success, or error describing failure:
//   - Connection errors
//   - Protocol errors
//   - Deserialization errors
//
// Example:
//
//	var response ResponseStruct
//	err := client.Request(
//	    context.Background(),
//	    "Service.Method",
//	    RequestParams{Field: "value"},
//	    &response,
//	)
func (c *rpcClient) Request(ctx context.Context, method string, params, result any, opts ...jsonrpc2.CallOption) error {
	conn, err := net.Dial("unix", c.sockPath)
	if err != nil {
		return err
	}
	if !c.keepLive {
		defer conn.Close()
	}

	// Generate monotonic request ID
	c.idCounter++
	idOpt := jsonrpc2.PickID(jsonrpc2.ID{Num: uint64(c.idCounter)})
	opts = append(opts, idOpt)

	client := c.adapter.NewConn(ctx, conn)
	return client.Request(ctx, method, params, result, opts...)
}

func (c *rpcClient) RequestEx(
	ctx context.Context,
	method string,
	params any,
	onResponse ResponseHandler,
	header Header,
	opts ...jsonrpc2.CallOption,
) error {
	normalizedHeader, err := normalizeHeader(header)
	if err != nil {
		return err
	}

	switch normalizedHeader.Mode {
	case CallModeUnary:
		return c.requestExUnary(ctx, method, params, onResponse)
	case CallModeStream:
		return c.requestExStream(ctx, method, params, onResponse, &normalizedHeader)
	default:
		return fmt.Errorf("unsupported request mode: %d", normalizedHeader.Mode)
	}
}

func (c *rpcClient) requestExUnary(
	ctx context.Context,
	method string,
	params any,
	onResponse ResponseHandler,
) error {
	var raw map[string]any
	if err := c.Request(ctx, method, params, &raw); err != nil {
		return err
	}

	frame := mapUnaryResultToFrame(raw)

	if onResponse != nil {
		return onResponse(frame)
	}
	return nil
}

func (c *rpcClient) requestExStream(
	ctx context.Context,
	method string,
	params any,
	onResponse ResponseHandler,
	header *Header,
) error {
	conn, err := net.Dial("unix", c.sockPath)
	if err != nil {
		return err
	}
	if !c.keepLive {
		defer conn.Close()
	}

	client := NewFrameStreamClient(conn, c.streamCodec)
	return client.RequestExStream(ctx, method, params, onResponse, header)
}

// Header 归一化
func normalizeHeader(header Header) (Header, error) {
	if header.Version == 0 {
		header.Version = ProtocolVersion
	}
	if header.Mode == 0 {
		header.Mode = CallModeUnary
	}

	switch header.Mode {
	case CallModeUnary, CallModeStream:
		return header, nil
	default:
		return header, fmt.Errorf("invalid header mode: %d", header.Mode)
	}
}

// 老 unary Response -> StreamFrame
func mapUnaryResultToFrame(raw map[string]any) StreamFrame {
	frame := StreamFrame{
		Code:   200,
		Msg:    "OK",
		Data:   nil,
		Stream: false,
		Done:   true,
	}

	if raw == nil {
		return frame
	}

	if v, ok := raw["code"].(float64); ok {
		frame.Code = int(v)
	}
	if v, ok := raw["msg"].(string); ok && v != "" {
		frame.Msg = v
	}
	if v, ok := raw["data"]; ok {
		frame.Data = v
	}
	if v, ok := raw["meta"].(map[string]any); ok {
		frame.Meta = v
	}

	return frame
}
