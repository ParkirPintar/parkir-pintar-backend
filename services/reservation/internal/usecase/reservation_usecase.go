package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/parkir-pintar/reservation/internal/adapter"
	"github.com/parkir-pintar/reservation/internal/model"
	"github.com/parkir-pintar/reservation/internal/repository"
	"github.com/rs/zerolog/log"
)

// getEnvDuration reads an env var as seconds and returns a time.Duration.
func getEnvDuration(key string, defaultSeconds int) time.Duration {
	if v := os.Getenv(key); v != "" {
		if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
			return time.Duration(sec) * time.Second
		}
	}
	return time.Duration(defaultSeconds) * time.Second
}

// ReservationUsecase defines the business logic interface for reservations.
type ReservationUsecase interface {
	CreateReservation(ctx context.Context, driverID, mode, vehicleType, spotID, idempotencyKey string) (*model.Reservation, error)
	HoldSpot(ctx context.Context, spotID, driverID string) (time.Time, error)
	CancelReservation(ctx context.Context, reservationID string) (int64, error)
	CheckIn(ctx context.Context, reservationID, actualSpotID string) (*model.Reservation, bool, int64, error)
	GetReservation(ctx context.Context, reservationID string) (*model.Reservation, error)
}

type reservationUsecase struct {
	repo      repository.ReservationRepository
	search    adapter.SearchClient
	billing   adapter.BillingClient
	publisher adapter.EventPublisher
}

// NewReservationUsecase creates a ReservationUsecase with all required dependencies.
func NewReservationUsecase(
	repo repository.ReservationRepository,
	search adapter.SearchClient,
	billing adapter.BillingClient,
	publisher adapter.EventPublisher,
) ReservationUsecase {
	return &reservationUsecase{
		repo:      repo,
		search:    search,
		billing:   billing,
		publisher: publisher,
	}
}

// CreateReservation handles both system-assigned and user-selected reservation modes.
// It validates inputs, checks idempotency, and publishes a booking message to RabbitMQ
// for serial processing by the queue worker.
func (u *reservationUsecase) CreateReservation(ctx context.Context, driverID, mode, vehicleType, spotID, idempotencyKey string) (*model.Reservation, error) {
	// Idempotency check: return existing reservation if key was already processed
	if existing, err := u.repo.GetIdempotency(ctx, idempotencyKey); err == nil && existing != "" {
		return u.repo.GetByID(ctx, existing)
	}

	switch model.ReservationMode(mode) {
	case model.ModeSystemAssigned:
		return u.createSystemAssigned(ctx, driverID, vehicleType, idempotencyKey)
	case model.ModeUserSelected:
		return u.createUserSelected(ctx, driverID, spotID, vehicleType, idempotencyKey)
	default:
		return nil, fmt.Errorf("invalid reservation mode: %s", mode)
	}
}

// createSystemAssigned calls Search.GetFirstAvailable, pre-validates spot availability
// from Redis cache, then publishes a booking message to the consistent-hash exchange.
func (u *reservationUsecase) createSystemAssigned(ctx context.Context, driverID, vehicleType, idempotencyKey string) (*model.Reservation, error) {
	// Call Search Service to get the first available spot
	spotID, err := u.search.GetFirstAvailable(ctx, vehicleType)
	if err != nil {
		return nil, fmt.Errorf("no available spots: %w", err)
	}

	// Pre-validate spot availability from Redis cache (Requirement 20.4)
	// Check if the spot is already locked — if so, it's likely taken
	if owner, _ := u.repo.GetHoldOwner(ctx, spotID); owner != "" {
		// Spot is held by someone, try to get another
		return nil, fmt.Errorf("spot %s is currently held, try again", spotID)
	}

	// Publish booking message to RabbitMQ for serial processing
	bm := BookingMessage{
		DriverID:       driverID,
		SpotID:         spotID,
		Mode:           string(model.ModeSystemAssigned),
		VehicleType:    vehicleType,
		IdempotencyKey: idempotencyKey,
	}
	payload, err := json.Marshal(bm)
	if err != nil {
		return nil, fmt.Errorf("marshal booking message: %w", err)
	}

	if err := u.publisher.PublishBooking(ctx, spotID, payload); err != nil {
		return nil, fmt.Errorf("publish booking: %w", err)
	}

	log.Info().
		Str("driver_id", driverID).
		Str("spot_id", spotID).
		Str("mode", "SYSTEM_ASSIGNED").
		Msg("booking message published")

	// Return a pending reservation — the queue worker will create the actual record.
	// The caller should poll or wait for the reservation.confirmed event.
	return &model.Reservation{
		DriverID:       driverID,
		SpotID:         spotID,
		Mode:           model.ModeSystemAssigned,
		Status:         model.StatusReserved,
		BookingFee:     5000,
		IdempotencyKey: idempotencyKey,
	}, nil
}

// createUserSelected validates hold ownership, then publishes a booking message.
func (u *reservationUsecase) createUserSelected(ctx context.Context, driverID, spotID, vehicleType, idempotencyKey string) (*model.Reservation, error) {
	// Validate hold ownership (Requirement 7.1)
	holdOwner, err := u.repo.GetHoldOwner(ctx, spotID)
	if err != nil {
		return nil, fmt.Errorf("check hold: %w", err)
	}
	if holdOwner == "" {
		return nil, fmt.Errorf("HOLD_EXPIRED: no active hold on spot %s", spotID)
	}
	if holdOwner != driverID {
		return nil, fmt.Errorf("HOLD_EXPIRED: spot %s is held by another driver", spotID)
	}

	// Publish booking message to RabbitMQ for serial processing
	bm := BookingMessage{
		DriverID:       driverID,
		SpotID:         spotID,
		Mode:           string(model.ModeUserSelected),
		VehicleType:    vehicleType,
		IdempotencyKey: idempotencyKey,
	}
	payload, err := json.Marshal(bm)
	if err != nil {
		return nil, fmt.Errorf("marshal booking message: %w", err)
	}

	if err := u.publisher.PublishBooking(ctx, spotID, payload); err != nil {
		return nil, fmt.Errorf("publish booking: %w", err)
	}

	log.Info().
		Str("driver_id", driverID).
		Str("spot_id", spotID).
		Str("mode", "USER_SELECTED").
		Msg("booking message published")

	return &model.Reservation{
		DriverID:       driverID,
		SpotID:         spotID,
		Mode:           model.ModeUserSelected,
		Status:         model.StatusReserved,
		BookingFee:     5000,
		IdempotencyKey: idempotencyKey,
	}, nil
}

// HoldSpot acquires a temporary hold on a spot for user-selected mode.
func (u *reservationUsecase) HoldSpot(ctx context.Context, spotID, driverID string) (time.Time, error) {
	if err := u.repo.HoldSpot(ctx, spotID, driverID); err != nil {
		return time.Time{}, err
	}
	holdTTL := getEnvDuration("HOLD_TTL_SECONDS", 10)
	return time.Now().Add(holdTTL), nil
}

// CancelReservation cancels a reservation, releases the Redis lock, records
// the cancellation fee via Billing, and publishes a reservation.cancelled event.
func (u *reservationUsecase) CancelReservation(ctx context.Context, reservationID string) (int64, error) {
	res, err := u.repo.GetByID(ctx, reservationID)
	if err != nil {
		return 0, fmt.Errorf("get reservation: %w", err)
	}

	// Verify status is RESERVED (Requirement 9.4)
	if res.Status != model.StatusReserved {
		return 0, fmt.Errorf("FAILED_PRECONDITION: reservation status is %s, expected RESERVED", res.Status)
	}

	// Calculate cancellation fee based on elapsed time (Requirements 9.1, 9.2)
	elapsed := time.Since(res.ConfirmedAt).Minutes()
	var fee int64
	if elapsed > 2 {
		fee = 5000
	}

	// Update status to CANCELLED
	if err := u.repo.UpdateStatus(ctx, reservationID, model.StatusCancelled); err != nil {
		return 0, fmt.Errorf("update status: %w", err)
	}

	// Release Redis lock (Requirement 9.3, 19.3)
	if err := u.repo.ReleaseLock(ctx, res.SpotID); err != nil {
		log.Error().Err(err).Str("spot_id", res.SpotID).Msg("failed to release lock on cancel")
	}

	// Call Billing to record cancellation fee (Requirement 9.3)
	if fee > 0 {
		if err := u.billing.ApplyPenalty(ctx, reservationID, "cancellation", fee); err != nil {
			log.Error().Err(err).Str("reservation_id", reservationID).Msg("failed to record cancellation fee")
		}
	}

	// Publish reservation.cancelled event (Requirement 9.3)
	event := map[string]interface{}{
		"event_type":       "reservation.cancelled",
		"reservation_id":   reservationID,
		"driver_id":        res.DriverID,
		"spot_id":          res.SpotID,
		"cancellation_fee": fee,
		"cancelled_at":     time.Now().Format(time.RFC3339),
	}
	eventPayload, _ := json.Marshal(event)
	if err := u.publisher.PublishEvent(ctx, "reservation.cancelled", eventPayload); err != nil {
		log.Error().Err(err).Msg("failed to publish reservation.cancelled event")
	}

	log.Info().
		Str("reservation_id", reservationID).
		Int64("cancellation_fee", fee).
		Msg("reservation cancelled")

	return fee, nil
}

// CheckIn processes a driver check-in. It verifies the reservation status,
// sets checkin_at, and releases the Redis lock. Wrong-spot is BLOCKED by
// Presence Service before this method is called.
func (u *reservationUsecase) CheckIn(ctx context.Context, reservationID, actualSpotID string) (*model.Reservation, bool, int64, error) {
	res, err := u.repo.GetByID(ctx, reservationID)
	if err != nil {
		return nil, false, 0, fmt.Errorf("get reservation: %w", err)
	}

	// Verify status is RESERVED (Requirement 8.4)
	if res.Status != model.StatusReserved {
		return nil, false, 0, fmt.Errorf("FAILED_PRECONDITION: reservation status is %s, expected RESERVED", res.Status)
	}

	// Set checkin_at and update status to ACTIVE (Requirement 8.2)
	now := time.Now()
	if err := u.repo.SetCheckinAt(ctx, reservationID, now); err != nil {
		return nil, false, 0, fmt.Errorf("set checkin_at: %w", err)
	}
	res.CheckinAt = &now
	res.Status = model.StatusActive

	// Release Redis lock since the spot is now physically occupied (Requirement 19.4)
	if err := u.repo.ReleaseLock(ctx, res.SpotID); err != nil {
		log.Error().Err(err).Str("spot_id", res.SpotID).Msg("failed to release lock on checkin")
	}

	// Detect wrong spot — BLOCKED, not penalized
	wrongSpot := res.SpotID != actualSpotID

	if wrongSpot {
		// Wrong-spot is BLOCKED — do not allow check-in, do not start billing
		return res, true, 0, fmt.Errorf("FAILED_PRECONDITION: BLOCKED — must park at assigned spot %s, not %s", res.SpotID, actualSpotID)
	}

	// Note: Billing session is started by Presence Service, not here.

	// Publish event (Requirement 8.5)
	eventType := "checkin.confirmed"
	event := map[string]interface{}{
		"event_type":     eventType,
		"reservation_id": reservationID,
		"driver_id":      res.DriverID,
		"spot_id":        res.SpotID,
		"actual_spot_id": actualSpotID,
		"wrong_spot":     false,
		"checkin_at":     now.Format(time.RFC3339),
	}
	eventPayload, _ := json.Marshal(event)
	if err := u.publisher.PublishEvent(ctx, eventType, eventPayload); err != nil {
		log.Error().Err(err).Str("event_type", eventType).Msg("failed to publish checkin event")
	}

	log.Info().
		Str("reservation_id", reservationID).
		Msg("check-in processed")

	return res, false, 0, nil
}

// GetReservation retrieves a reservation by ID.
func (u *reservationUsecase) GetReservation(ctx context.Context, reservationID string) (*model.Reservation, error) {
	return u.repo.GetByID(ctx, reservationID)
}
