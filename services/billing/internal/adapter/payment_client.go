package adapter

import (
	"context"
	"fmt"

	paymentpb "github.com/parkir-pintar/payment/pkg/proto"
	"google.golang.org/grpc"
)

// PaymentClient abstracts calls to the Payment Service gRPC API.
type PaymentClient interface {
	CreatePayment(ctx context.Context, invoiceID string, amount int64, idempotencyKey string) (paymentID, qrCode string, err error)
}

type paymentClient struct {
	conn grpc.ClientConnInterface
}

// NewPaymentClient creates a PaymentClient backed by the given gRPC connection.
func NewPaymentClient(conn grpc.ClientConnInterface) PaymentClient {
	return &paymentClient{conn: conn}
}

func (c *paymentClient) CreatePayment(ctx context.Context, invoiceID string, amount int64, idempotencyKey string) (string, string, error) {
	req := &paymentpb.CreatePaymentRequest{
		InvoiceId:      invoiceID,
		Amount:         amount,
		IdempotencyKey: idempotencyKey,
	}

	resp := &paymentpb.PaymentResponse{}
	err := c.conn.Invoke(ctx, "/payment.PaymentService/CreatePayment", req, resp)
	if err != nil {
		return "", "", fmt.Errorf("payment CreatePayment: %w", err)
	}

	return resp.PaymentId, resp.QrCode, nil
}
