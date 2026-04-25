package adapter

import (
	"context"
	"fmt"
	"testing"
	"time"

	billingpb "github.com/parkir-pintar/billing/pkg/proto"
	"google.golang.org/grpc"
)

// fakeBillingConn implements grpc.ClientConnInterface for testing.
type fakeBillingConn struct {
	resp   *billingpb.BillingResponse
	err    error
	method string // captures the last invoked method
}

func (f *fakeBillingConn) Invoke(ctx context.Context, method string, args any, reply any, opts ...grpc.CallOption) error {
	f.method = method
	if f.err != nil {
		return f.err
	}
	out := reply.(*billingpb.BillingResponse)
	*out = *f.resp
	return nil
}

func (f *fakeBillingConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestChargeBookingFee_Success(t *testing.T) {
	conn := &fakeBillingConn{
		resp: &billingpb.BillingResponse{
			BillingId: "bill-123",
			Status:    "PENDING",
		},
	}
	client := NewBillingClient(conn)

	err := client.ChargeBookingFee(context.Background(), "res-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.method != "/billing.BillingService/ChargeBookingFee" {
		t.Errorf("method = %q, want %q", conn.method, "/billing.BillingService/ChargeBookingFee")
	}
}

func TestChargeBookingFee_Error(t *testing.T) {
	conn := &fakeBillingConn{
		err: fmt.Errorf("connection refused"),
	}
	client := NewBillingClient(conn)

	err := client.ChargeBookingFee(context.Background(), "res-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestApplyPenalty_Success(t *testing.T) {
	conn := &fakeBillingConn{
		resp: &billingpb.BillingResponse{
			BillingId: "bill-123",
			Status:    "PENDING",
		},
	}
	client := NewBillingClient(conn)

	err := client.ApplyPenalty(context.Background(), "res-1", "WRONG_SPOT", 200000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.method != "/billing.BillingService/ApplyPenalty" {
		t.Errorf("method = %q, want %q", conn.method, "/billing.BillingService/ApplyPenalty")
	}
}

func TestApplyPenalty_Error(t *testing.T) {
	conn := &fakeBillingConn{
		err: fmt.Errorf("connection refused"),
	}
	client := NewBillingClient(conn)

	err := client.ApplyPenalty(context.Background(), "res-1", "WRONG_SPOT", 200000)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestStartBillingSession_Success(t *testing.T) {
	conn := &fakeBillingConn{
		resp: &billingpb.BillingResponse{
			BillingId: "bill-123",
			Status:    "ACTIVE",
		},
	}
	client := NewBillingClient(conn)

	checkinAt := time.Date(2025, 7, 15, 10, 30, 0, 0, time.UTC)
	err := client.StartBillingSession(context.Background(), "res-1", checkinAt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.method != "/billing.BillingService/StartBillingSession" {
		t.Errorf("method = %q, want %q", conn.method, "/billing.BillingService/StartBillingSession")
	}
}

func TestStartBillingSession_Error(t *testing.T) {
	conn := &fakeBillingConn{
		err: fmt.Errorf("connection refused"),
	}
	client := NewBillingClient(conn)

	err := client.StartBillingSession(context.Background(), "res-1", time.Now())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
