package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/parkir-pintar/billing/internal/adapter"
	"github.com/parkir-pintar/billing/internal/model"
	"github.com/parkir-pintar/billing/internal/repository"
	"github.com/rs/zerolog/log"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type BillingUsecase interface {
	ChargeBookingFee(ctx context.Context, reservationID string) (*model.BillingRecord, error)
	StartSession(ctx context.Context, reservationID string, checkinAt time.Time) error
	ApplyPenalty(ctx context.Context, reservationID string, reason string, amount int64) error
	Checkout(ctx context.Context, reservationID, idempotencyKey string) (*model.BillingRecord, error)
}

type billingUsecase struct {
	repo          repository.BillingRepository
	paymentClient adapter.PaymentClient
	publisher     adapter.EventPublisher
	mu            sync.RWMutex
	ruleVersion   int
	engine        *PricingEngine
}

func NewBillingUsecase(ctx context.Context, repo repository.BillingRepository, paymentClient adapter.PaymentClient, publisher adapter.EventPublisher) (BillingUsecase, error) {
	// Load JDM rules from file first, then from DB — fail if neither available.
	var ruleContent []byte
	if data, err := os.ReadFile(envOr("PRICING_RULES_PATH", "rules/pricing.json")); err == nil {
		ruleContent = data
		log.Info().Msg("loaded JDM pricing rules from file")
	} else {
		// Try loading from DB
		if content, _, err := repo.GetActivePricingRule(ctx); err == nil && len(content) > 0 {
			ruleContent = content
			log.Info().Msg("loaded JDM pricing rules from database")
		}
	}

	engine, err := NewPricingEngine(ruleContent)
	if err != nil {
		return nil, fmt.Errorf("pricing engine initialization failed: %w", err)
	}

	uc := &billingUsecase{
		repo:          repo,
		paymentClient: paymentClient,
		publisher:     publisher,
		engine:        engine,
	}
	go uc.hotReload(ctx)
	return uc, nil
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
			content, version, err := u.repo.GetActivePricingRule(ctx)
			if err != nil {
				log.Error().Err(err).Msg("failed to poll pricing rules")
				continue
			}
			u.mu.Lock()
			if version != u.ruleVersion {
				newEngine, err := NewPricingEngine(content)
				if err != nil {
					log.Error().Err(err).Int("version", version).Msg("failed to reload pricing rules, keeping current engine")
					u.mu.Unlock()
					continue
				}
				u.ruleVersion = version
				oldEngine := u.engine
				u.engine = newEngine
				if oldEngine != nil {
					oldEngine.Dispose()
				}
				log.Info().Int("version", version).Msg("pricing rules reloaded from DB")
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
		Total:         5000,
		Status:        model.BillingPending,
	}
	if err := u.repo.Create(ctx, b); err != nil {
		return nil, err
	}

	// Call Payment Service to generate QRIS QR code for booking fee
	if u.paymentClient != nil {
		paymentID, qrCode, err := u.paymentClient.CreatePayment(ctx, b.ID, b.BookingFee, fmt.Sprintf("booking-%s", reservationID))
		if err != nil {
			log.Error().Err(err).
				Str("reservation_id", reservationID).
				Msg("booking fee payment creation failed")
			b.Status = model.BillingFailed
			_ = u.repo.Update(ctx, b)
			return b, fmt.Errorf("create booking fee payment: %w", err)
		}
		b.PaymentID = paymentID
		b.QRCode = qrCode
		_ = u.repo.Update(ctx, b)
	}

	return b, nil
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

	// No-show has no additional penalty — driver only forfeits the booking fee.
	if reason == "noshow" {
		return nil
	}

	b.Penalty += amount
	b.Total += amount

	return u.repo.Update(ctx, b)
}

func (u *billingUsecase) Checkout(ctx context.Context, reservationID, idempotencyKey string) (*model.BillingRecord, error) {
	// Step 1: Idempotency check — return cached record if key already exists.
	if idempotencyKey != "" {
		existing, err := u.repo.GetByIdempotencyKey(ctx, idempotencyKey)
		if err != nil {
			return nil, fmt.Errorf("idempotency check: %w", err)
		}
		if existing != nil {
			log.Debug().
				Str("idempotency_key", idempotencyKey).
				Str("invoice_id", existing.ID).
				Msg("returning cached checkout result")
			return existing, nil
		}
	}

	// Step 2: Get billing record by reservation_id.
	b, err := u.repo.GetByReservationID(ctx, reservationID)
	if err != nil {
		return nil, fmt.Errorf("billing record not found for reservation %s: %w", reservationID, err)
	}

	// Step 3: Calculate session end time (now).
	now := time.Now()
	b.SessionEnd = &now

	// Step 4: Evaluate pricing with all flags from the existing record.
	u.mu.RLock()
	engine := u.engine
	u.mu.RUnlock()

	var durationHours float64
	if b.SessionStart != nil {
		durationHours = now.Sub(*b.SessionStart).Hours()
	}

	var midnightCrossings int
	if b.SessionStart != nil {
		midnightCrossings = countMidnightCrossings(*b.SessionStart, now)
	}

	input := model.PricingInput{
		DurationHours:     durationHours,
		MidnightCrossings: midnightCrossings,
		BookingFee:        b.BookingFee,
	}
	output, evalErr := engine.Evaluate(input)
	if evalErr != nil {
		return nil, fmt.Errorf("pricing evaluation failed: %w", evalErr)
	}

	b.HourlyFee = output.HourlyFee
	b.OvernightFee = output.OvernightFee
	b.CancelFee = output.CancellationFee
	// Option B: Booking fee acts as a deposit — deduct from checkout total.
	// Driver already paid booking_fee at reservation time, so net checkout amount
	// is the remaining balance after subtracting the deposit.
	grossTotal := output.HourlyFee + output.OvernightFee + output.CancellationFee
	if grossTotal > b.BookingFee {
		b.Total = grossTotal - b.BookingFee
	} else {
		// Booking fee covers the entire session — nothing more to pay.
		b.Total = 0
	}
	b.IdempotencyKey = idempotencyKey

	// Step 5: Call Payment.CreatePayment to get QR code and payment_id.
	// Skip payment if total is 0 (e.g. session not started, or instant checkout).
	if u.paymentClient != nil && b.Total > 0 {
		paymentID, qrCode, payErr := u.paymentClient.CreatePayment(ctx, b.ID, b.Total, idempotencyKey)
		if payErr != nil {
			log.Error().Err(payErr).
				Str("invoice_id", b.ID).
				Msg("payment creation failed")

			b.Status = model.BillingFailed

			// Persist the failed billing record.
			if updateErr := u.repo.Update(ctx, b); updateErr != nil {
				log.Error().Err(updateErr).Msg("failed to update billing record after payment failure")
			}

			// Publish checkout.failed event.
			u.publishEvent(ctx, "checkout.failed", b)

			return nil, fmt.Errorf("create payment: %w", payErr)
		}

		b.PaymentID = paymentID
		b.QRCode = qrCode
	}

	// Step 6: Update billing record with all fees, payment_id, qr_code.
	if err := u.repo.Update(ctx, b); err != nil {
		return nil, fmt.Errorf("update billing record: %w", err)
	}

	// Step 7: Store idempotency key for future dedup.
	if idempotencyKey != "" {
		if err := u.repo.SetIdempotencyKey(ctx, idempotencyKey, b.ID); err != nil {
			log.Error().Err(err).
				Str("idempotency_key", idempotencyKey).
				Msg("failed to store idempotency key (non-fatal)")
		}
	}

	// Step 8: Publish checkout.completed event.
	u.publishEvent(ctx, "checkout.completed", b)

	return b, nil
}

// publishEvent publishes a domain event to RabbitMQ. It is a best-effort
// operation — failures are logged but do not fail the checkout.
func (u *billingUsecase) publishEvent(ctx context.Context, eventType string, b *model.BillingRecord) {
	if u.publisher == nil {
		return
	}

	payload, err := json.Marshal(map[string]any{
		"event_type":       eventType,
		"invoice_id":       b.ID,
		"reservation_id":   b.ReservationID,
		"booking_fee":      b.BookingFee,
		"hourly_fee":       b.HourlyFee,
		"overnight_fee":    b.OvernightFee,
		"penalty":          b.Penalty,
		"cancellation_fee": b.CancelFee,
		"total":            b.Total,
		"amount":           b.Total,
		"status":           string(b.Status),
		"payment_id":       b.PaymentID,
		"qr_code":          b.QRCode,
	})
	if err != nil {
		log.Error().Err(err).Str("event", eventType).Msg("failed to marshal event payload")
		return
	}

	if pubErr := u.publisher.Publish(ctx, eventType, payload); pubErr != nil {
		log.Error().Err(pubErr).Str("event", eventType).Msg("failed to publish event")
	}
}
