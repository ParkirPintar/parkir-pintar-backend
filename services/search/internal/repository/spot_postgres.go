package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parkir-pintar/search/internal/model"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

const cacheTTL = 10 * time.Second

type spotRepo struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func NewSpotRepository(db *pgxpool.Pool, redis *redis.Client) SpotRepository {
	return &spotRepo{db: db, redis: redis}
}

func (r *spotRepo) GetAvailableSpots(ctx context.Context, floor int, vehicleType string) ([]model.Spot, error) {
	cacheKey := fmt.Sprintf("availability:%d:%s", floor, vehicleType)

	// Try Redis cache first
	cached, err := r.redis.Get(ctx, cacheKey).Bytes()
	if err == nil {
		var spots []model.Spot
		if unmarshalErr := json.Unmarshal(cached, &spots); unmarshalErr == nil {
			log.Debug().Str("key", cacheKey).Msg("cache hit")
			return spots, nil
		} else {
			log.Warn().Str("key", cacheKey).Err(unmarshalErr).Msg("cache deserialize error, falling through to DB")
		}
	} else if err != redis.Nil {
		// Redis connection error — fall through to DB
		log.Warn().Str("key", cacheKey).Err(err).Msg("redis error, falling through to DB")
	} else {
		log.Debug().Str("key", cacheKey).Msg("cache miss")
	}

	// Query PostgreSQL
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

	// Populate Redis cache
	if data, marshalErr := json.Marshal(spots); marshalErr == nil {
		if setErr := r.redis.Set(ctx, cacheKey, data, cacheTTL).Err(); setErr != nil {
			log.Warn().Str("key", cacheKey).Err(setErr).Msg("failed to set cache")
		}
	}

	return spots, nil
}

func (r *spotRepo) GetFirstAvailable(ctx context.Context, vehicleType string) (*model.Spot, error) {
	cacheKey := fmt.Sprintf("availability:%s", vehicleType)

	// Try Redis cache first
	cached, err := r.redis.Get(ctx, cacheKey).Bytes()
	if err == nil {
		var s model.Spot
		if unmarshalErr := json.Unmarshal(cached, &s); unmarshalErr == nil {
			log.Debug().Str("key", cacheKey).Msg("cache hit")
			return &s, nil
		} else {
			log.Warn().Str("key", cacheKey).Err(unmarshalErr).Msg("cache deserialize error, falling through to DB")
		}
	} else if err != redis.Nil {
		log.Warn().Str("key", cacheKey).Err(err).Msg("redis error, falling through to DB")
	} else {
		log.Debug().Str("key", cacheKey).Msg("cache miss")
	}

	// Query PostgreSQL
	var s model.Spot
	err = r.db.QueryRow(ctx,
		`SELECT spot_id, floor, vehicle_type, status FROM spots WHERE vehicle_type=$1 AND status='AVAILABLE' LIMIT 1`,
		vehicleType,
	).Scan(&s.SpotID, &s.Floor, &s.VehicleType, &s.Status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("no available spot for vehicle type %s", vehicleType)
		}
		return nil, err
	}

	// Populate Redis cache
	if data, marshalErr := json.Marshal(s); marshalErr == nil {
		if setErr := r.redis.Set(ctx, cacheKey, data, cacheTTL).Err(); setErr != nil {
			log.Warn().Str("key", cacheKey).Err(setErr).Msg("failed to set cache")
		}
	}

	return &s, nil
}
