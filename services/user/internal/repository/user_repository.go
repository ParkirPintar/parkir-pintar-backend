package repository

import (
	"context"

	"github.com/parkir-pintar/user/internal/model"
)

type UserRepository interface {
	Create(ctx context.Context, u *model.User) error
	GetByID(ctx context.Context, id string) (*model.User, error)
	GetByLicensePlate(ctx context.Context, licensePlate, vehicleType string) (*model.User, error)
	Update(ctx context.Context, u *model.User) error
	SetTokenBlacklist(ctx context.Context, token string) error
	IsTokenBlacklisted(ctx context.Context, token string) (bool, error)
}
