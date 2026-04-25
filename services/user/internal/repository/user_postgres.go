package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parkir-pintar/user/internal/model"
	"github.com/redis/go-redis/v9"
)

const refreshTokenTTL = 7 * 24 * time.Hour // 7 days

type userRepository struct {
	db  *pgxpool.Pool
	rdb *redis.Client
}

func NewUserRepository(db *pgxpool.Pool, rdb *redis.Client) UserRepository {
	return &userRepository{db: db, rdb: rdb}
}

func (r *userRepository) Create(ctx context.Context, u *model.User) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO users (id, license_plate, vehicle_type, password_hash, name, phone_number, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now(), now())`,
		u.ID, u.LicensePlate, u.VehicleType, u.PasswordHash, u.Name, u.PhoneNumber,
	)
	return err
}

func (r *userRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	u := &model.User{}
	err := r.db.QueryRow(ctx,
		`SELECT id, license_plate, vehicle_type, password_hash, name, phone_number, created_at, updated_at FROM users WHERE id=$1`, id,
	).Scan(&u.ID, &u.LicensePlate, &u.VehicleType, &u.PasswordHash, &u.Name, &u.PhoneNumber, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (r *userRepository) GetByLicensePlate(ctx context.Context, licensePlate, vehicleType string) (*model.User, error) {
	u := &model.User{}
	err := r.db.QueryRow(ctx,
		`SELECT id, license_plate, vehicle_type, password_hash, name, phone_number, created_at, updated_at FROM users WHERE license_plate=$1 AND vehicle_type=$2`,
		licensePlate, vehicleType,
	).Scan(&u.ID, &u.LicensePlate, &u.VehicleType, &u.PasswordHash, &u.Name, &u.PhoneNumber, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (r *userRepository) Update(ctx context.Context, u *model.User) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET name=$1, phone_number=$2, updated_at=now() WHERE id=$3`,
		u.Name, u.PhoneNumber, u.ID,
	)
	return err
}

func (r *userRepository) SetTokenBlacklist(ctx context.Context, jti string, ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("invalid TTL for blacklist: %v", ttl)
	}
	return r.rdb.Set(ctx, "blacklist:"+jti, "1", ttl).Err()
}

func (r *userRepository) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	n, err := r.rdb.Exists(ctx, "blacklist:"+jti).Result()
	return n > 0, err
}

func (r *userRepository) StoreRefreshToken(ctx context.Context, token string, driverID string) error {
	return r.rdb.Set(ctx, "refresh:"+token, driverID, refreshTokenTTL).Err()
}

func (r *userRepository) GetRefreshToken(ctx context.Context, token string) (string, error) {
	driverID, err := r.rdb.Get(ctx, "refresh:"+token).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("refresh token not found or expired")
	}
	return driverID, err
}

func (r *userRepository) DeleteRefreshToken(ctx context.Context, token string) error {
	return r.rdb.Del(ctx, "refresh:"+token).Err()
}
