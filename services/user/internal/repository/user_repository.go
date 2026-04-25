package repository

import (
	"context"
	"time"

	"github.com/parkir-pintar/user/internal/model"
)

type UserRepository interface {
	Create(ctx context.Context, u *model.User) error
	GetByID(ctx context.Context, id string) (*model.User, error)
	GetByLicensePlate(ctx context.Context, licensePlate, vehicleType string) (*model.User, error)
	Update(ctx context.Context, u *model.User) error
	SetTokenBlacklist(ctx context.Context, jti string, ttl time.Duration) error
	IsTokenBlacklisted(ctx context.Context, jti string) (bool, error)
	StoreRefreshToken(ctx context.Context, token string, driverID string) error
	GetRefreshToken(ctx context.Context, token string) (string, error)
	DeleteRefreshToken(ctx context.Context, token string) error
}
