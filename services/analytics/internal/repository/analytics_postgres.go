package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parkir-pintar/analytics/internal/model"
)

type analyticsRepo struct {
	db *pgxpool.Pool
}

func NewAnalyticsRepository(db *pgxpool.Pool) AnalyticsRepository {
	return &analyticsRepo{db: db}
}

func (r *analyticsRepo) Save(ctx context.Context, event *model.TransactionEvent) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO transaction_events (id, event_type, reservation_id, driver_id, spot_id, vehicle_type, amount, payload, recorded_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		event.ID, event.EventType, event.ReservationID, event.DriverID, event.SpotID, event.VehicleType, event.Amount, event.Payload, time.Now(),
	)
	return err
}
