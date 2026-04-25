package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/parkir-pintar/reservation/internal/model"
	"github.com/parkir-pintar/reservation/internal/repository"
)

type ReservationUsecase interface {
	CreateReservation(ctx context.Context, driverID, mode, vehicleType, spotID, idempotencyKey string) (*model.Reservation, error)
	HoldSpot(ctx context.Context, spotID, driverID string) (time.Time, error)
	CancelReservation(ctx context.Context, reservationID string) (int64, error)
	CheckIn(ctx context.Context, reservationID, actualSpotID string) (*model.Reservation, bool, int64, error)
	GetReservation(ctx context.Context, reservationID string) (*model.Reservation, error)
}

type reservationUsecase struct {
	repo repository.ReservationRepository
}

func NewReservationUsecase(repo repository.ReservationRepository) ReservationUsecase {
	return &reservationUsecase{repo: repo}
}

func (u *reservationUsecase) CreateReservation(ctx context.Context, driverID, mode, vehicleType, spotID, idempotencyKey string) (*model.Reservation, error) {
	if existing, err := u.repo.GetIdempotency(ctx, idempotencyKey); err == nil && existing != "" {
		return u.repo.GetByID(ctx, existing)
	}

	if err := u.repo.LockSpot(ctx, spotID); err != nil {
		return nil, fmt.Errorf("spot unavailable: %w", err)
	}

	now := time.Now()
	res := &model.Reservation{
		ID:             uuid.NewString(),
		DriverID:       driverID,
		SpotID:         spotID,
		Mode:           model.ReservationMode(mode),
		Status:         model.StatusReserved,
		BookingFee:     5000,
		ConfirmedAt:    now,
		ExpiresAt:      now.Add(1 * time.Hour),
		IdempotencyKey: idempotencyKey,
	}
	if err := u.repo.Create(ctx, res); err != nil {
		return nil, err
	}
	_ = u.repo.SetIdempotency(ctx, idempotencyKey, res.ID)
	return res, nil
}

func (u *reservationUsecase) HoldSpot(ctx context.Context, spotID, driverID string) (time.Time, error) {
	if err := u.repo.HoldSpot(ctx, spotID, driverID); err != nil {
		return time.Time{}, err
	}
	return time.Now().Add(60 * time.Second), nil
}

func (u *reservationUsecase) CancelReservation(ctx context.Context, reservationID string) (int64, error) {
	res, err := u.repo.GetByID(ctx, reservationID)
	if err != nil {
		return 0, err
	}
	elapsed := time.Since(res.ConfirmedAt).Minutes()
	var fee int64
	if elapsed > 2 {
		fee = 5000
	}
	if err := u.repo.UpdateStatus(ctx, reservationID, model.StatusCancelled); err != nil {
		return 0, err
	}
	_ = u.repo.ReleaseHold(ctx, res.SpotID)
	return fee, nil
}

func (u *reservationUsecase) CheckIn(ctx context.Context, reservationID, actualSpotID string) (*model.Reservation, bool, int64, error) {
	res, err := u.repo.GetByID(ctx, reservationID)
	if err != nil {
		return nil, false, 0, err
	}
	wrongSpot := res.SpotID != actualSpotID
	var penalty int64
	if wrongSpot {
		penalty = 200000
	}
	if err := u.repo.UpdateStatus(ctx, reservationID, model.StatusActive); err != nil {
		return nil, false, 0, err
	}
	return res, wrongSpot, penalty, nil
}

func (u *reservationUsecase) GetReservation(ctx context.Context, reservationID string) (*model.Reservation, error) {
	return u.repo.GetByID(ctx, reservationID)
}
