package usecase

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/parkir-pintar/reservation/internal/adapter"
	"github.com/parkir-pintar/reservation/internal/model"
	"github.com/parkir-pintar/reservation/internal/repository"
	"github.com/rs/zerolog/log"
)

const (
	// expiryInterval is how often the expiry worker scans for expired reservations.
	expiryInterval = 30 * time.Second

	// defaultNoshowFee is the fallback no-show fee if gorules JSON cannot be read.
	defaultNoshowFee int64 = 5000
)

// getNoshowFeeFromRules reads the no-show fee from the gorules pricing JSON file.
// This keeps the pricing.json as the single source of truth for all fee values.
// Falls back to defaultNoshowFee (5000 IDR) if the file cannot be read or parsed.
func getNoshowFeeFromRules() int64 {
	rulesPath := os.Getenv("PRICING_RULES_PATH")
	if rulesPath == "" {
		rulesPath = "rules/pricing.json"
	}

	data, err := os.ReadFile(rulesPath)
	if err != nil {
		log.Warn().Err(err).Str("path", rulesPath).Msg("cannot read pricing rules, using default noshow fee")
		return defaultNoshowFee
	}

	var rules struct {
		Nodes []struct {
			ID      string `json:"id"`
			Content struct {
				Rules []map[string]string `json:"rules"`
			} `json:"content"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(data, &rules); err != nil {
		log.Warn().Err(err).Msg("cannot parse pricing rules, using default noshow fee")
		return defaultNoshowFee
	}

	// Find the noshow-fee-table node and extract the fee from the first rule
	for _, node := range rules.Nodes {
		if node.ID == "noshow-fee-table" {
			for _, rule := range node.Content.Rules {
				if val, ok := rule["ns_o1"]; ok && val != "0" {
					var fee int64
					if err := json.Unmarshal([]byte(val), &fee); err == nil && fee > 0 {
						return fee
					}
				}
			}
		}
	}

	return defaultNoshowFee
}

// ExpiryWorker periodically scans for expired reservations and processes them.
type ExpiryWorker struct {
	repo      repository.ReservationRepository
	billing   adapter.BillingClient
	publisher adapter.EventPublisher
	interval  time.Duration
}

// NewExpiryWorker creates an ExpiryWorker with the given dependencies.
func NewExpiryWorker(
	repo repository.ReservationRepository,
	billing adapter.BillingClient,
	publisher adapter.EventPublisher,
) *ExpiryWorker {
	return &ExpiryWorker{
		repo:      repo,
		billing:   billing,
		publisher: publisher,
		interval:  expiryInterval,
	}
}

// Start begins the periodic expiry scan. It blocks until the context is cancelled.
func (w *ExpiryWorker) Start(ctx context.Context) error {
	log.Info().Dur("interval", w.interval).Msg("expiry worker started")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("expiry worker stopping")
			return ctx.Err()
		case <-ticker.C:
			w.scan(ctx)
		}
	}
}

// scan queries for expired reservations and processes each one.
func (w *ExpiryWorker) scan(ctx context.Context) {
	expired, err := w.repo.GetExpiredReservations(ctx)
	if err != nil {
		log.Error().Err(err).Msg("expiry worker: failed to get expired reservations")
		return
	}

	if len(expired) == 0 {
		return
	}

	log.Info().Int("count", len(expired)).Msg("expiry worker: processing expired reservations")

	for _, res := range expired {
		w.processExpired(ctx, res)
	}
}

// processExpired handles a single expired reservation:
// 1. Skip if status is ACTIVE or COMPLETED (Requirement 10.3)
// 2. Update status to EXPIRED (Requirement 10.1)
// 3. Apply no-show fee via Billing (Requirement 10.2)
// 4. Release Redis lock (Requirement 10.2)
// 5. Publish reservation.expired event (Requirement 10.4)
func (w *ExpiryWorker) processExpired(ctx context.Context, res *model.Reservation) {
	logger := log.With().
		Str("reservation_id", res.ID).
		Str("spot_id", res.SpotID).
		Str("driver_id", res.DriverID).
		Logger()

	// Skip if reservation is no longer in RESERVED status (Requirement 10.3)
	// This handles the case where the driver checked in between the query and processing.
	if res.Status == model.StatusActive || res.Status == model.StatusCompleted {
		logger.Debug().Str("status", string(res.Status)).Msg("skipping non-RESERVED reservation")
		return
	}

	// Update status to EXPIRED (Requirement 10.1)
	if err := w.repo.UpdateStatus(ctx, res.ID, model.StatusExpired); err != nil {
		logger.Error().Err(err).Msg("failed to update reservation to EXPIRED")
		return
	}

	// Apply no-show fee via Billing Service (Requirement 10.2)
	// Fee value is read from gorules pricing.json (single source of truth)
	fee := getNoshowFeeFromRules()
	if err := w.billing.ApplyPenalty(ctx, res.ID, "noshow", fee); err != nil {
		logger.Error().Err(err).Msg("failed to apply no-show fee")
	}

	// Release Redis lock to free the spot (Requirement 10.2)
	if err := w.repo.ReleaseLock(ctx, res.SpotID); err != nil {
		logger.Error().Err(err).Msg("failed to release lock for expired reservation")
	}

	// Publish reservation.expired event (Requirement 10.4)
	event := map[string]interface{}{
		"event_type":     "reservation.expired",
		"reservation_id": res.ID,
		"driver_id":      res.DriverID,
		"spot_id":        res.SpotID,
		"noshow_fee":     fee,
		"expired_at":     time.Now().Format(time.RFC3339),
	}
	eventPayload, _ := json.Marshal(event)
	if err := w.publisher.PublishEvent(ctx, "reservation.expired", eventPayload); err != nil {
		logger.Error().Err(err).Msg("failed to publish reservation.expired event")
	}

	logger.Info().Msg("reservation expired and processed")
}
