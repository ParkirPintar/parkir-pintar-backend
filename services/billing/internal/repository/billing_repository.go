package repository

import (
	"context"

	"github.com/parkir-pintar/billing/internal/model"
)

type BillingRepository interface {
	Create(ctx context.Context, b *model.BillingRecord) error
	GetByReservationID(ctx context.Context, reservationID string) (*model.BillingRecord, error)
	Update(ctx context.Context, b *model.BillingRecord) error
	GetActivePricingRule(ctx context.Context) ([]byte, int, error) // returns JDM content + version
	GetByIdempotencyKey(ctx context.Context, key string) (*model.BillingRecord, error)
	SetIdempotencyKey(ctx context.Context, key string, invoiceID string) error
}
