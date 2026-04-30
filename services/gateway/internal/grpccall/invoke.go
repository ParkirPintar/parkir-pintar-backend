// Package grpccall provides a generic JSON-over-gRPC invoker.
// All backend services use a JSON codec registered as "proto",
// so we send/receive raw JSON bytes over the gRPC wire.
package grpccall

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
)

func init() {
	encoding.RegisterCodec(jsonCodec{})
}

type jsonCodec struct{}

func (jsonCodec) Marshal(v interface{}) ([]byte, error)   { return json.Marshal(v) }
func (jsonCodec) Unmarshal(data []byte, v interface{}) error { return json.Unmarshal(data, v) }
func (jsonCodec) Name() string                             { return "proto" }

// Invoke calls a gRPC method with JSON request/response.
// method format: "/package.Service/Method"
func Invoke(ctx context.Context, conn *grpc.ClientConn, method string, req, resp interface{}) error {
	err := conn.Invoke(ctx, method, req, resp)
	if err != nil {
		return fmt.Errorf("grpc %s: %w", method, err)
	}
	return nil
}
