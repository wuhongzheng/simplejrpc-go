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
	sockPath   string        // Path to the Unix domain socket
	adapter    ClientAdapter // Protocol adapter (defaults to JSON-RPC)
	frameCodec FrameCodec    // Frame frameCodec
	idCounter  int64         // Atomic counter for generating request IDs
	keepLive   bool          // Connection persistence flag
}

// NewRpcKeepLiveClient creates a new RPC client with connection persistence disabled.
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
		sockPath:   socketPath,
		adapter:    &JsonRpcSimpleClient{},
		frameCodec: &LengthFrameCodec{},
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
		sockPath:   socketPath,
		adapter:    &JsonRpcSimpleClient{},
		frameCodec: &LengthFrameCodec{},
		keepLive:   true,
	}
}

// Request executes a JSON-RPC 2.0 method call and handles response decoding.
// Automatically manages connection establishment, request ID generation, and error handling.
//
// This method is protocol-generic from the client perspective: the caller provides result,
// and decoding follows the actual JSON-RPC response shape returned by the server.
// When used against this project's default legacy JSON-RPC server implementation,
// the response payload is wrapped in Response.
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

// RequestEx Extend support for stream through custom protocols
//
// Routing behavior:
//   - CallModeUnary: uses the legacy JSON-RPC Request path
//   - CallModeStream: uses the new frame stream protocol
//
// Note:
//   - The unary compatibility path is intentionally tied to this project's legacy server contract.
//   - That legacy unary contract returns a Response envelope, which is then converted to a StreamFrame.
//   - Request itself remains generic; only RequestEx unary compatibility assumes the legacy Response shape.
//
// Parameters:
//   - ctx:       Context for cancellation and timeout control
//   - method:    RPC method name to invoke
//   - params:    Input parameters
//   - onResponse ResponseHandler for handling streaming responses
//   - header:    Custom request header
//
// Returns:
//   - error:     nil on success, or error describing failure:
func (c *rpcClient) RequestEx(
	ctx context.Context,
	method string,
	params any,
	onResponse ResponseHandler,
	header Header,
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
		return fmt.Errorf("invalid header mode: %d", normalizedHeader.Mode)
	}
}

func (c *rpcClient) requestExUnary(
	ctx context.Context,
	method string,
	params any,
	onResponse ResponseHandler,
) error {
	// The legacy unary compatibility path targets this project's default JSON-RPC server,
	// whose result payload is wrapped in Response.
	var result Response
	if err := c.Request(ctx, method, params, &result); err != nil {
		return err
	}

	if onResponse == nil {
		return nil
	}

	frame := StreamFrame{
		Code:   result.Code,
		Msg:    result.Message,
		Stream: false,
		Data:   result.Data,
		Done:   true,
	}
	if result.Meta != nil {
		frame.Meta = map[string]any{
			"endpoint": result.Meta.Endpoint,
			"close":    result.Meta.Close,
		}
	}

	return onResponse(frame)
}

// requestExStream handles the actual request processing for RequestEx.
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

	client := NewFrameClient(conn, c.frameCodec)
	return client.RequestWithFrame(ctx, method, params, onResponse, header)
}

// normalizeHeader applies default protocol values and validates the request mode.
//
// Defaults:
//   - Version: ProtocolVersion
//   - Mode:    CallModeUnary (default)
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
