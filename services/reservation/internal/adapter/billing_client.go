package adapter

import (
	"context"
	"fmt"
	"time"

	billingpb "github.com/parkir-pintar/billing/pkg/proto"
	"google.golang.org/grpc"
)

// BillingClient abstracts calls to the Billing Service gRPC API.
type BillingClient interface {
	ChargeBookingFee(ctx context.Context, reservationID string) (paymentID, qrCode string, err error)
	ApplyPenalty(ctx context.Context, reservationID, reason string, amount int64) error
	StartBillingSession(ctx context.Context, reservationID string, checkinAt time.Time) error
}

type billingClient struct {
	conn grpc.ClientConnInterface
}

// NewBillingClient creates a BillingClient backed by the given gRPC connection.
func NewBillingClient(conn grpc.ClientConnInterface) BillingClient {
	return &billingClient{conn: conn}
}

func (c *billingClient) ChargeBookingFee(ctx context.Context, reservationID string) (string, string, error) {
	req := &billingpb.ChargeBookingFeeRequest{
		ReservationId: reservationID,
		Amount:        5000,
	}

	resp := &billingpb.BillingResponse{}
	err := c.conn.Invoke(ctx, "/billing.BillingService/ChargeBookingFee", req, resp)
	if err != nil {
		return "", "", fmt.Errorf("billing ChargeBookingFee: %w", err)
	}

	return resp.PaymentId, resp.QrCode, nil
}

func (c *billingClient) ApplyPenalty(ctx context.Context, reservationID, reason string, amount int64) error {
	req := &billingpb.ApplyPenaltyRequest{
		ReservationId: reservationID,
		Reason:        reason,
		Amount:        amount,
	}

	resp := &billingpb.BillingResponse{}
	err := c.conn.Invoke(ctx, "/billing.BillingService/ApplyPenalty", req, resp)
	if err != nil {
		return fmt.Errorf("billing ApplyPenalty: %w", err)
	}

	return nil
}

func (c *billingClient) StartBillingSession(ctx context.Context, reservationID string, checkinAt time.Time) error {
	req := &billingpb.StartBillingSessionRequest{
		ReservationId: reservationID,
		CheckinAt:     checkinAt.Format(time.RFC3339),
	}

	resp := &billingpb.BillingResponse{}
	err := c.conn.Invoke(ctx, "/billing.BillingService/StartBillingSession", req, resp)
	if err != nil {
		return fmt.Errorf("billing StartBillingSession: %w", err)
	}

	return nil
}
