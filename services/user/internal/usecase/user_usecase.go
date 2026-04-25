package usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/parkir-pintar/user/internal/model"
	"github.com/parkir-pintar/user/internal/repository"
)

type UserUsecase interface {
	Register(ctx context.Context, licensePlate, vehicleType, name, phone string) (*model.User, error)
	GetProfile(ctx context.Context, userID string) (*model.User, error)
	UpdateProfile(ctx context.Context, userID, name, phone string) (*model.User, error)
	Authenticate(ctx context.Context, licensePlate, vehicleType string) (*model.User, error)
}

type userUsecase struct {
	repo repository.UserRepository
}

func NewUserUsecase(repo repository.UserRepository) UserUsecase {
	return &userUsecase{repo: repo}
}

func (u *userUsecase) Register(ctx context.Context, licensePlate, vehicleType, name, phone string) (*model.User, error) {
	if existing, _ := u.repo.GetByLicensePlate(ctx, licensePlate, vehicleType); existing != nil {
		return nil, fmt.Errorf("duplicate license plate + vehicle type")
	}
	user := &model.User{
		ID:           uuid.NewString(),
		LicensePlate: licensePlate,
		VehicleType:  vehicleType,
		Name:         name,
		PhoneNumber:  phone,
	}
	if err := u.repo.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (u *userUsecase) GetProfile(ctx context.Context, userID string) (*model.User, error) {
	return u.repo.GetByID(ctx, userID)
}

func (u *userUsecase) UpdateProfile(ctx context.Context, userID, name, phone string) (*model.User, error) {
	user, err := u.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.Name = name
	user.PhoneNumber = phone
	if err := u.repo.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (u *userUsecase) Authenticate(ctx context.Context, licensePlate, vehicleType string) (*model.User, error) {
	user, err := u.repo.GetByLicensePlate(ctx, licensePlate, vehicleType)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}
	return user, nil
}
