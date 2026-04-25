package adapter

import (
	"context"
	"fmt"
	"testing"

	searchpb "github.com/parkir-pintar/search/pkg/proto"
	"google.golang.org/grpc"
)

// fakeSearchConn implements grpc.ClientConnInterface for testing.
type fakeSearchConn struct {
	resp *searchpb.GetFirstAvailableResponse
	err  error
}

func (f *fakeSearchConn) Invoke(ctx context.Context, method string, args any, reply any, opts ...grpc.CallOption) error {
	if f.err != nil {
		return f.err
	}
	out := reply.(*searchpb.GetFirstAvailableResponse)
	*out = *f.resp
	return nil
}

func (f *fakeSearchConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestGetFirstAvailable_Success(t *testing.T) {
	conn := &fakeSearchConn{
		resp: &searchpb.GetFirstAvailableResponse{
			SpotId: "1-CAR-01",
		},
	}
	client := NewSearchClient(conn)

	spotID, err := client.GetFirstAvailable(context.Background(), "CAR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spotID != "1-CAR-01" {
		t.Errorf("spotID = %q, want %q", spotID, "1-CAR-01")
	}
}

func TestGetFirstAvailable_Error(t *testing.T) {
	conn := &fakeSearchConn{
		err: fmt.Errorf("connection refused"),
	}
	client := NewSearchClient(conn)

	_, err := client.GetFirstAvailable(context.Background(), "CAR")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetFirstAvailable_MotorcycleType(t *testing.T) {
	conn := &fakeSearchConn{
		resp: &searchpb.GetFirstAvailableResponse{
			SpotId: "3-MOTO-25",
		},
	}
	client := NewSearchClient(conn)

	spotID, err := client.GetFirstAvailable(context.Background(), "MOTORCYCLE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spotID != "3-MOTO-25" {
		t.Errorf("spotID = %q, want %q", spotID, "3-MOTO-25")
	}
}
