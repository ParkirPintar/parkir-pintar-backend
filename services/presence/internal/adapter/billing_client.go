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
	StartBillingSession(ctx context.Context, reservationID string, checkinAt time.Time) error
}

type billingClient struct {
	conn grpc.ClientConnInterface
}

// NewBillingClient creates a BillingClient backed by the given gRPC connection.
func NewBillingClient(conn grpc.ClientConnInterface) BillingClient {
	return &billingClient{conn: conn}
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
