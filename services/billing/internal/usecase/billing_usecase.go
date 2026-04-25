package usecase

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/parkir-pintar/billing/internal/model"
	"github.com/parkir-pintar/billing/internal/repository"
	"github.com/rs/zerolog/log"
)

type BillingUsecase interface {
	ChargeBookingFee(ctx context.Context, reservationID string) (*model.BillingRecord, error)
	StartSession(ctx context.Context, reservationID string, checkinAt time.Time) error
	ApplyPenalty(ctx context.Context, reservationID string, reason string, amount int64) error
	Checkout(ctx context.Context, reservationID, idempotencyKey string) (*model.BillingRecord, error)
}

type billingUsecase struct {
	repo           repository.BillingRepository
	mu             sync.RWMutex
	ruleVersion    int
	// engine      *gorules.Engine  // TODO: wire gorules engine
}

func NewBillingUsecase(ctx context.Context, repo repository.BillingRepository) BillingUsecase {
	uc := &billingUsecase{repo: repo}
	go uc.hotReload(ctx)
	return uc
}

// hotReload polls DB every 30s and reloads pricing rules when version changes.
func (u *billingUsecase) hotReload(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, version, err := u.repo.GetActivePricingRule(ctx)
			if err != nil {
				log.Error().Err(err).Msg("failed to poll pricing rules")
				continue
			}
			u.mu.Lock()
			if version != u.ruleVersion {
				u.ruleVersion = version
				// TODO: u.engine.Reload(content)
				log.Info().Int("version", version).Msg("pricing rules reloaded")
			}
			u.mu.Unlock()
		}
	}
}

func (u *billingUsecase) ChargeBookingFee(ctx context.Context, reservationID string) (*model.BillingRecord, error) {
	b := &model.BillingRecord{
		ID:            uuid.NewString(),
		ReservationID: reservationID,
		BookingFee:    5000,
		Status:        model.BillingPending,
	}
	return b, u.repo.Create(ctx, b)
}

func (u *billingUsecase) StartSession(ctx context.Context, reservationID string, checkinAt time.Time) error {
	b, err := u.repo.GetByReservationID(ctx, reservationID)
	if err != nil {
		return err
	}
	b.SessionStart = &checkinAt
	return u.repo.Update(ctx, b)
}

func (u *billingUsecase) ApplyPenalty(ctx context.Context, reservationID, reason string, amount int64) error {
	b, err := u.repo.GetByReservationID(ctx, reservationID)
	if err != nil {
		return err
	}
	b.Penalty += amount
	return u.repo.Update(ctx, b)
}

func (u *billingUsecase) Checkout(ctx context.Context, reservationID, idempotencyKey string) (*model.BillingRecord, error) {
	b, err := u.repo.GetByReservationID(ctx, reservationID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	b.SessionEnd = &now

	// TODO: replace with gorules engine evaluation
	input := model.PricingInput{
		DurationHours:   now.Sub(*b.SessionStart).Hours(),
		CrossesMidnight: crossesMidnight(*b.SessionStart, now),
	}
	output := evaluatePricing(input)

	b.HourlyFee = output.HourlyFee
	b.OvernightFee = output.OvernightFee
	b.Total = b.BookingFee + b.HourlyFee + b.OvernightFee + b.Penalty + b.NoshowFee + b.CancelFee

	return b, u.repo.Update(ctx, b)
}

// evaluatePricing is a fallback pure-Go pricing engine (gorules replaces this).
func evaluatePricing(in model.PricingInput) model.PricingOutput {
	out := model.PricingOutput{BookingFee: 5000}
	out.HourlyFee = int64(math.Ceil(in.DurationHours)) * 5000
	if in.CrossesMidnight {
		out.OvernightFee = 20000
	}
	if in.WrongSpot {
		out.Penalty = 200000
	}
	if in.IsNoshow {
		out.Penalty = 10000
	}
	if in.CancelElapsedMinutes > 2 {
		out.CancellationFee = 5000
	}
	out.Total = out.BookingFee + out.HourlyFee + out.OvernightFee + out.Penalty + out.CancellationFee
	return out
}

func crossesMidnight(start, end time.Time) bool {
	startDay := start.Truncate(24 * time.Hour)
	endDay := end.Truncate(24 * time.Hour)
	return endDay.After(startDay)
}
