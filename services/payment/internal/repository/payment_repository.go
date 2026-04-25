package repository

import (
	"context"

	"github.com/parkir-pintar/payment/internal/model"
)

type PaymentRepository interface {
	Create(ctx context.Context, p *model.Payment) error
	GetByID(ctx context.Context, id string) (*model.Payment, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*model.Payment, error)
	UpdateStatus(ctx context.Context, id string, status model.PaymentStatus) error
}
