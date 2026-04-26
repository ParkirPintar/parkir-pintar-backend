package proto

import (
	"encoding/json"
	"fmt"

	"google.golang.org/grpc/encoding"
)

// jsonCodec implements grpc encoding.Codec using JSON.
// This allows hand-written Go structs (instead of protobuf-generated types)
// to be used as gRPC request/response messages.
type jsonCodec struct{}

func init() {
	encoding.RegisterCodec(jsonCodec{})
}

func (jsonCodec) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (jsonCodec) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func (jsonCodec) Name() string {
	return "proto"
}

func (jsonCodec) String() string {
	return fmt.Sprintf("json-as-proto codec")
}
