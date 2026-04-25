package adapter

import (
	"context"
	"fmt"
	"testing"

	paymentpb "github.com/parkir-pintar/payment/pkg/proto"
	"google.golang.org/grpc"
)

// fakeConn implements grpc.ClientConnInterface for testing.
type fakeConn struct {
	resp *paymentpb.PaymentResponse
	err  error
}

func (f *fakeConn) Invoke(ctx context.Context, method string, args any, reply any, opts ...grpc.CallOption) error {
	if f.err != nil {
		return f.err
	}
	out := reply.(*paymentpb.PaymentResponse)
	*out = *f.resp
	return nil
}

func (f *fakeConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestCreatePayment_Success(t *testing.T) {
	conn := &fakeConn{
		resp: &paymentpb.PaymentResponse{
			PaymentId: "pay-123",
			QrCode:    "qr-abc",
		},
	}
	client := NewPaymentClient(conn)

	paymentID, qrCode, err := client.CreatePayment(context.Background(), "inv-1", 50000, "key-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if paymentID != "pay-123" {
		t.Errorf("paymentID = %q, want %q", paymentID, "pay-123")
	}
	if qrCode != "qr-abc" {
		t.Errorf("qrCode = %q, want %q", qrCode, "qr-abc")
	}
}

func TestCreatePayment_Error(t *testing.T) {
	conn := &fakeConn{
		err: fmt.Errorf("connection refused"),
	}
	client := NewPaymentClient(conn)

	_, _, err := client.CreatePayment(context.Background(), "inv-1", 50000, "key-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
