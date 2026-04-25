package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/parkir-pintar/payment/internal/model"
	"github.com/parkir-pintar/payment/internal/repository"
	"github.com/sony/gobreaker/v2"
)

type PaymentUsecase interface {
	CreatePayment(ctx context.Context, invoiceID string, amount int64, idempotencyKey string) (*model.Payment, error)
	GetPaymentStatus(ctx context.Context, paymentID string) (*model.Payment, error)
	RetryPayment(ctx context.Context, paymentID, idempotencyKey string) (*model.Payment, error)
}

type paymentUsecase struct {
	repo       repository.PaymentRepository
	cb         *gobreaker.CircuitBreaker[*model.Payment]
	settlement settlementClient
}

// settlementClient is the interface for the external Pondo Ngopi settlement stub.
type settlementClient interface {
	RequestQRIS(ctx context.Context, invoiceID string, amount int64) (string, error)
	CheckStatus(ctx context.Context, paymentID string) (model.PaymentStatus, error)
}

func NewPaymentUsecase(repo repository.PaymentRepository, settlement settlementClient) PaymentUsecase {
	cb := gobreaker.NewCircuitBreaker[*model.Payment](gobreaker.Settings{
		Name:        "settlement",
		MaxRequests: 3,
		Interval:    10 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	})
	return &paymentUsecase{repo: repo, cb: cb, settlement: settlement}
}

func (u *paymentUsecase) CreatePayment(ctx context.Context, invoiceID string, amount int64, idempotencyKey string) (*model.Payment, error) {
	// Idempotency check: return existing payment if one exists for this key.
	if idempotencyKey != "" {
		existing, err := u.repo.GetByIdempotencyKey(ctx, idempotencyKey)
		if err == nil && existing != nil {
			return existing, nil
		}
		// If error is "no rows" or similar, continue to create a new payment.
	}

	p, err := u.cb.Execute(func() (*model.Payment, error) {
		qr, err := u.settlement.RequestQRIS(ctx, invoiceID, amount)
		if err != nil {
			return nil, fmt.Errorf("settlement QRIS: %w", err)
		}
		payment := &model.Payment{
			ID:             uuid.NewString(),
			InvoiceID:      invoiceID,
			Amount:         amount,
			Status:         model.PaymentPending,
			Method:         "QRIS",
			QRCode:         qr,
			IdempotencyKey: idempotencyKey,
		}
		return payment, u.repo.Create(ctx, payment)
	})

	// Circuit breaker OPEN state fallback: return a PENDING payment without QR code.
	if err != nil && errors.Is(err, gobreaker.ErrOpenState) {
		payment := &model.Payment{
			ID:             uuid.NewString(),
			InvoiceID:      invoiceID,
			Amount:         amount,
			Status:         model.PaymentPending,
			Method:         "QRIS",
			QRCode:         "",
			IdempotencyKey: idempotencyKey,
		}
		if createErr := u.repo.Create(ctx, payment); createErr != nil {
			return nil, fmt.Errorf("create fallback payment: %w", createErr)
		}
		return payment, nil
	}

	return p, err
}

func (u *paymentUsecase) GetPaymentStatus(ctx context.Context, paymentID string) (*model.Payment, error) {
	p, err := u.repo.GetByID(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	latest, err := u.settlement.CheckStatus(ctx, paymentID)
	if err == nil && latest != p.Status {
		_ = u.repo.UpdateStatus(ctx, paymentID, latest)
		p.Status = latest
	}
	return p, nil
}

func (u *paymentUsecase) RetryPayment(ctx context.Context, paymentID, idempotencyKey string) (*model.Payment, error) {
	p, err := u.repo.GetByID(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	return u.CreatePayment(ctx, p.InvoiceID, p.Amount, idempotencyKey)
}
