package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parkir-pintar/reservation/internal/model"
	"github.com/redis/go-redis/v9"
)

const (
	holdTTL        = 60 * time.Second
	lockTTL        = 1 * time.Hour
	idempotencyTTL = 24 * time.Hour
)

type reservationRepo struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func NewReservationRepository(db *pgxpool.Pool, rdb *redis.Client) ReservationRepository {
	return &reservationRepo{db: db, redis: rdb}
}

func (r *reservationRepo) Create(ctx context.Context, res *model.Reservation) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO reservations (id, driver_id, spot_id, mode, status, booking_fee, confirmed_at, expires_at, idempotency_key)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		res.ID, res.DriverID, res.SpotID, res.Mode, res.Status,
		res.BookingFee, res.ConfirmedAt, res.ExpiresAt, res.IdempotencyKey,
	)
	return err
}

func (r *reservationRepo) GetByID(ctx context.Context, id string) (*model.Reservation, error) {
	var res model.Reservation
	err := r.db.QueryRow(ctx,
		`SELECT id, driver_id, spot_id, mode, status, booking_fee, confirmed_at, expires_at, checkin_at, idempotency_key
		 FROM reservations WHERE id=$1`, id,
	).Scan(&res.ID, &res.DriverID, &res.SpotID, &res.Mode, &res.Status,
		&res.BookingFee, &res.ConfirmedAt, &res.ExpiresAt, &res.CheckinAt, &res.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (r *reservationRepo) UpdateStatus(ctx context.Context, id string, status model.ReservationStatus) error {
	_, err := r.db.Exec(ctx, `UPDATE reservations SET status=$1 WHERE id=$2`, status, id)
	return err
}

func (r *reservationRepo) GetIdempotency(ctx context.Context, key string) (string, error) {
	return r.redis.Get(ctx, fmt.Sprintf("idempotency:%s", key)).Result()
}

func (r *reservationRepo) SetIdempotency(ctx context.Context, key, reservationID string) error {
	return r.redis.Set(ctx, fmt.Sprintf("idempotency:%s", key), reservationID, idempotencyTTL).Err()
}

func (r *reservationRepo) HoldSpot(ctx context.Context, spotID, driverID string) error {
	ok, err := r.redis.SetNX(ctx, fmt.Sprintf("hold:%s", spotID), driverID, holdTTL).Result()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("spot %s already held", spotID)
	}
	return nil
}

func (r *reservationRepo) ReleaseHold(ctx context.Context, spotID string) error {
	return r.redis.Del(ctx, fmt.Sprintf("hold:%s", spotID)).Err()
}

func (r *reservationRepo) LockSpot(ctx context.Context, spotID string) error {
	ok, err := r.redis.SetNX(ctx, fmt.Sprintf("lock:%s", spotID), "1", lockTTL).Result()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("spot %s already locked", spotID)
	}
	return nil
}
