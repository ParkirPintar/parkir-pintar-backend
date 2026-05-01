package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/parkir-pintar/reservation/internal/adapter"
	"github.com/parkir-pintar/reservation/internal/model"
	"github.com/parkir-pintar/reservation/internal/repository"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
)

// BookingMessage represents a message published to the booking exchange.
type BookingMessage struct {
	DriverID       string `json:"driver_id"`
	SpotID         string `json:"spot_id"`
	Mode           string `json:"mode"`
	VehicleType    string `json:"vehicle_type"`
	IdempotencyKey string `json:"idempotency_key"`
}

// QueueWorker consumes booking messages from RabbitMQ and processes them
// serially per spot (guaranteed by consistent-hash exchange routing).
type QueueWorker struct {
	repo      repository.ReservationRepository
	billing   adapter.BillingClient
	search    adapter.SearchClient
	publisher adapter.EventPublisher
	ch        *amqp.Channel
	queueName string
}

// NewQueueWorker creates a QueueWorker that consumes from the given queue.
func NewQueueWorker(
	repo repository.ReservationRepository,
	billing adapter.BillingClient,
	search adapter.SearchClient,
	publisher adapter.EventPublisher,
	ch *amqp.Channel,
	queueName string,
) *QueueWorker {
	return &QueueWorker{
		repo:      repo,
		billing:   billing,
		search:    search,
		publisher: publisher,
		ch:        ch,
		queueName: queueName,
	}
}

// Start begins consuming messages from the booking queue. It blocks until
// the context is cancelled or the channel is closed.
func (w *QueueWorker) Start(ctx context.Context) error {
	// Declare queue and bind to booking exchange
	if _, err := w.ch.QueueDeclare(w.queueName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("queue worker declare %s: %w", w.queueName, err)
	}
	if err := w.ch.QueueBind(w.queueName, "10", "booking.exchange", false, nil); err != nil {
		log.Warn().Err(err).Msg("queue bind failed (exchange may not exist yet)")
	}
	log.Info().Str("queue", w.queueName).Msg("queue declared and bound")

	msgs, err := w.ch.Consume(
		w.queueName, // queue
		"",          // consumer tag (auto-generated)
		false,       // auto-ack disabled — we ack/nack manually
		false,       // exclusive
		false,       // no-local
		false,       // no-wait
		nil,         // args
	)
	if err != nil {
		return fmt.Errorf("queue worker consume %s: %w", w.queueName, err)
	}

	log.Info().Str("queue", w.queueName).Msg("queue worker started")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("queue worker stopping")
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				log.Warn().Msg("queue worker channel closed")
				return nil
			}
			w.processMessage(ctx, msg)
		}
	}
}

func (w *QueueWorker) processMessage(ctx context.Context, msg amqp.Delivery) {
	var bm BookingMessage
	if err := json.Unmarshal(msg.Body, &bm); err != nil {
		log.Error().Err(err).Msg("queue worker: invalid message payload")
		_ = msg.Nack(false, false) // discard malformed messages
		return
	}

	logger := log.With().
		Str("driver_id", bm.DriverID).
		Str("spot_id", bm.SpotID).
		Str("mode", bm.Mode).
		Str("idempotency_key", bm.IdempotencyKey).
		Logger()

	// Idempotency check: if this key was already processed, ack and return
	if existing, err := w.repo.GetIdempotency(ctx, bm.IdempotencyKey); err == nil && existing != "" {
		logger.Info().Str("reservation_id", existing).Msg("idempotent duplicate, acking")
		_ = msg.Ack(false)
		return
	}

	// Acquire Redis lock for the spot
	if err := w.repo.LockSpot(ctx, bm.SpotID); err != nil {
		logger.Warn().Err(err).Msg("lock acquisition failed")

		if bm.Mode == string(model.ModeSystemAssigned) {
			// For system-assigned: nack for requeue so the message can be
			// retried (the usecase will pick a new spot on the next attempt)
			_ = msg.Nack(false, true)
		} else {
			// For user-selected: the specific spot is taken, nack without requeue
			_ = msg.Nack(false, false)
		}
		return
	}

	// Create reservation record
	now := time.Now()
	res := &model.Reservation{
		ID:             uuid.NewString(),
		DriverID:       bm.DriverID,
		SpotID:         bm.SpotID,
		Mode:           model.ReservationMode(bm.Mode),
		Status:         model.StatusReserved,
		BookingFee:     5000,
		ConfirmedAt:    now,
		ExpiresAt:      now.Add(1 * time.Hour),
		IdempotencyKey: bm.IdempotencyKey,
	}

	if err := w.repo.Create(ctx, res); err != nil {
		logger.Error().Err(err).Msg("failed to create reservation")
		// Release the lock since we couldn't create the reservation
		_ = w.repo.ReleaseLock(ctx, bm.SpotID)
		_ = msg.Nack(false, true)
		return
	}

	// Charge booking fee via Billing Service
	if err := w.billing.ChargeBookingFee(ctx, res.ID); err != nil {
		logger.Error().Err(err).Msg("failed to charge booking fee")
		// Reservation is created but billing failed — still ack to avoid
		// duplicate reservations. The fee can be reconciled later.
	}

	// Store idempotency key
	if err := w.repo.SetIdempotency(ctx, bm.IdempotencyKey, res.ID); err != nil {
		logger.Error().Err(err).Msg("failed to store idempotency key")
	}

	// If user-selected mode, release the hold since the lock is now in place
	if bm.Mode == string(model.ModeUserSelected) {
		_ = w.repo.ReleaseHold(ctx, bm.SpotID)
	}

	// Publish reservation.confirmed event
	event := map[string]interface{}{
		"event_type":     "reservation.confirmed",
		"reservation_id": res.ID,
		"driver_id":      res.DriverID,
		"spot_id":        res.SpotID,
		"vehicle_type":   bm.VehicleType,
		"mode":           bm.Mode,
		"booking_fee":    res.BookingFee,
		"confirmed_at":   res.ConfirmedAt.Format(time.RFC3339),
		"expires_at":     res.ExpiresAt.Format(time.RFC3339),
	}
	eventPayload, _ := json.Marshal(event)
	if err := w.publisher.PublishEvent(ctx, "reservation.confirmed", eventPayload); err != nil {
		logger.Error().Err(err).Msg("failed to publish reservation.confirmed event")
	}

	logger.Info().Str("reservation_id", res.ID).Msg("reservation confirmed")
	_ = msg.Ack(false)
}
