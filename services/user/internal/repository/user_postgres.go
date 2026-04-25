package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parkir-pintar/user/internal/model"
	"github.com/redis/go-redis/v9"
)

type userRepository struct {
	db  *pgxpool.Pool
	rdb *redis.Client
}

func NewUserRepository(db *pgxpool.Pool, rdb *redis.Client) UserRepository {
	return &userRepository{db: db, rdb: rdb}
}

func (r *userRepository) Create(ctx context.Context, u *model.User) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO users (id, license_plate, vehicle_type, name, phone_number, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, now(), now())`,
		u.ID, u.LicensePlate, u.VehicleType, u.Name, u.PhoneNumber,
	)
	return err
}

func (r *userRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	u := &model.User{}
	err := r.db.QueryRow(ctx,
		`SELECT id, license_plate, vehicle_type, name, phone_number, created_at, updated_at FROM users WHERE id=$1`, id,
	).Scan(&u.ID, &u.LicensePlate, &u.VehicleType, &u.Name, &u.PhoneNumber, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (r *userRepository) GetByLicensePlate(ctx context.Context, licensePlate, vehicleType string) (*model.User, error) {
	u := &model.User{}
	err := r.db.QueryRow(ctx,
		`SELECT id, license_plate, vehicle_type, name, phone_number, created_at, updated_at FROM users WHERE license_plate=$1 AND vehicle_type=$2`,
		licensePlate, vehicleType,
	).Scan(&u.ID, &u.LicensePlate, &u.VehicleType, &u.Name, &u.PhoneNumber, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (r *userRepository) Update(ctx context.Context, u *model.User) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET name=$1, phone_number=$2, updated_at=now() WHERE id=$3`,
		u.Name, u.PhoneNumber, u.ID,
	)
	return err
}

func (r *userRepository) SetTokenBlacklist(ctx context.Context, token string) error {
	return r.rdb.Set(ctx, "blacklist:"+token, 1, 24*time.Hour).Err()
}

func (r *userRepository) IsTokenBlacklisted(ctx context.Context, token string) (bool, error) {
	n, err := r.rdb.Exists(ctx, "blacklist:"+token).Result()
	return n > 0, err
}
