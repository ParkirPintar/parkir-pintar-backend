package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/parkir-pintar/presence/internal/model"
	"github.com/rs/zerolog/log"
)

// PresenceUsecase processes location updates and manages check-in/check-out.
type PresenceUsecase interface {
	// ProcessLocation receives a single location update from the driver app.
	// Returns a presence event with the driver's current position info.
	ProcessLocation(ctx context.Context, update model.LocationUpdate) (*model.PresenceEvent, error)

	// CheckIn processes a driver check-in at the parking gate.
	// Validates spot assignment, calls Reservation.CheckIn, then calls
	// Billing.StartBillingSession to start the billing timer.
	CheckIn(ctx context.Context, reservationID, actualSpotID string) (*model.CheckInResult, error)

	// CheckOut initiates the checkout flow for a driver leaving the parking area.
	CheckOut(ctx context.Context, reservationID string) error
}

// ReservationClient calls Reservation Service via gRPC.
type ReservationClient interface {
	CheckIn(ctx context.Context, reservationID, actualSpotID string) error
	GetReservation(ctx context.Context, reservationID string) (spotID string, err error)
}

// BillingClient calls Billing Service via gRPC.
type BillingClient interface {
	StartBillingSession(ctx context.Context, reservationID string, checkinAt time.Time) error
}

type presenceUsecase struct {
	reservation ReservationClient
	billing     BillingClient
}

// NewPresenceUsecase creates a PresenceUsecase with the given dependencies.
func NewPresenceUsecase(reservation ReservationClient, billing BillingClient) PresenceUsecase {
	return &presenceUsecase{
		reservation: reservation,
		billing:     billing,
	}
}

func (u *presenceUsecase) ProcessLocation(ctx context.Context, update model.LocationUpdate) (*model.PresenceEvent, error) {
	// Simple location tracking — no geofence evaluation.
	// The app sends location updates every ≤30s for tracking purposes.
	// Check-in is triggered explicitly via the CheckIn RPC, not by geofence.
	return &model.PresenceEvent{
		ReservationID: update.ReservationID,
		Event:         "LOCATION_UPDATED",
	}, nil
}

// CheckIn processes a driver check-in. It validates the spot assignment via
// Reservation Service, then triggers Billing.StartBillingSession to start
// the billing timer. Presence owns the check-in trigger.
func (u *presenceUsecase) CheckIn(ctx context.Context, reservationID, actualSpotID string) (*model.CheckInResult, error) {
	// Get the reserved spot to validate
	reservedSpotID, err := u.reservation.GetReservation(ctx, reservationID)
	if err != nil {
		return nil, fmt.Errorf("get reservation: %w", err)
	}

	// Wrong spot → BLOCKED
	if actualSpotID != reservedSpotID {
		log.Warn().
			Str("reservation_id", reservationID).
			Str("expected", reservedSpotID).
			Str("actual", actualSpotID).
			Msg("wrong spot — blocked")
		return &model.CheckInResult{
			ReservationID: reservationID,
			Status:        "BLOCKED",
			WrongSpot:     true,
		}, fmt.Errorf("BLOCKED: driver at spot %s, expected %s", actualSpotID, reservedSpotID)
	}

	// Correct spot → call Reservation.CheckIn to set status=ACTIVE
	if err := u.reservation.CheckIn(ctx, reservationID, actualSpotID); err != nil {
		return nil, fmt.Errorf("reservation check-in: %w", err)
	}

	// Presence triggers billing — call Billing.StartBillingSession
	now := time.Now()
	if err := u.billing.StartBillingSession(ctx, reservationID, now); err != nil {
		log.Error().Err(err).Str("reservation_id", reservationID).Msg("failed to start billing session (non-fatal)")
		// Non-fatal: check-in succeeded, billing can be retried
	}

	log.Info().
		Str("reservation_id", reservationID).
		Str("spot_id", actualSpotID).
		Msg("check-in confirmed, billing started")

	return &model.CheckInResult{
		ReservationID: reservationID,
		Status:        "ACTIVE",
		CheckinAt:     now.Format(time.RFC3339),
		WrongSpot:     false,
	}, nil
}

// CheckOut initiates the checkout flow. For now it logs the event;
// the actual billing/payment is handled by the driver calling /v1/checkout.
func (u *presenceUsecase) CheckOut(ctx context.Context, reservationID string) error {
	log.Info().Str("reservation_id", reservationID).Msg("check-out initiated via presence")
	return nil
}
