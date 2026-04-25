package repository

import (
	"context"
	"time"

	"github.com/parkir-pintar/reservation/internal/model"
)

type ReservationRepository interface {
	Create(ctx context.Context, r *model.Reservation) error
	GetByID(ctx context.Context, id string) (*model.Reservation, error)
	UpdateStatus(ctx context.Context, id string, status model.ReservationStatus) error
	GetIdempotency(ctx context.Context, key string) (string, error)
	SetIdempotency(ctx context.Context, key, reservationID string) error
	HoldSpot(ctx context.Context, spotID, driverID string) error
	ReleaseHold(ctx context.Context, spotID string) error
	LockSpot(ctx context.Context, spotID string) error
	ReleaseLock(ctx context.Context, spotID string) error
	GetHoldOwner(ctx context.Context, spotID string) (string, error)
	SetCheckinAt(ctx context.Context, id string, t time.Time) error
	GetExpiredReservations(ctx context.Context) ([]*model.Reservation, error)
}
