package repository

import (
	"context"

	"github.com/parkir-pintar/search/internal/model"
)

type SpotRepository interface {
	GetAvailableSpots(ctx context.Context, floor int, vehicleType string) ([]model.Spot, error)
	GetFirstAvailable(ctx context.Context, vehicleType string) (*model.Spot, error)
}
