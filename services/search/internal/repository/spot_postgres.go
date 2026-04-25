package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parkir-pintar/search/internal/model"
	"github.com/redis/go-redis/v9"
)

type spotRepo struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func NewSpotRepository(db *pgxpool.Pool, redis *redis.Client) SpotRepository {
	return &spotRepo{db: db, redis: redis}
}

func (r *spotRepo) GetAvailableSpots(ctx context.Context, floor int, vehicleType string) ([]model.Spot, error) {
	cacheKey := fmt.Sprintf("availability:%d:%s", floor, vehicleType)
	// TODO: check redis cache first, fallback to DB read replica
	_ = cacheKey
	rows, err := r.db.Query(ctx,
		`SELECT spot_id, floor, vehicle_type, status FROM spots WHERE floor=$1 AND vehicle_type=$2 AND status='AVAILABLE'`,
		floor, vehicleType,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var spots []model.Spot
	for rows.Next() {
		var s model.Spot
		if err := rows.Scan(&s.SpotID, &s.Floor, &s.VehicleType, &s.Status); err != nil {
			return nil, err
		}
		spots = append(spots, s)
	}
	return spots, nil
}

func (r *spotRepo) GetFirstAvailable(ctx context.Context, vehicleType string) (*model.Spot, error) {
	cacheKey := fmt.Sprintf("availability:%s", vehicleType)
	// TODO: check redis cache first
	_ = cacheKey
	var s model.Spot
	err := r.db.QueryRow(ctx,
		`SELECT spot_id, floor, vehicle_type, status FROM spots WHERE vehicle_type=$1 AND status='AVAILABLE' LIMIT 1`,
		vehicleType,
	).Scan(&s.SpotID, &s.Floor, &s.VehicleType, &s.Status)
	if err != nil {
		return nil, err
	}
	return &s, nil
}
