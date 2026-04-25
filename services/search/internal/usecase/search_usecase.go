package usecase

import (
	"context"

	"github.com/parkir-pintar/search/internal/model"
	"github.com/parkir-pintar/search/internal/repository"
)

type SearchUsecase interface {
	GetAvailability(ctx context.Context, floor int, vehicleType string) ([]model.Spot, error)
	GetFirstAvailable(ctx context.Context, vehicleType string) (*model.Spot, error)
}

type searchUsecase struct {
	repo repository.SpotRepository
}

func NewSearchUsecase(repo repository.SpotRepository) SearchUsecase {
	return &searchUsecase{repo: repo}
}

func (u *searchUsecase) GetAvailability(ctx context.Context, floor int, vehicleType string) ([]model.Spot, error) {
	return u.repo.GetAvailableSpots(ctx, floor, vehicleType)
}

func (u *searchUsecase) GetFirstAvailable(ctx context.Context, vehicleType string) (*model.Spot, error) {
	return u.repo.GetFirstAvailable(ctx, vehicleType)
}
