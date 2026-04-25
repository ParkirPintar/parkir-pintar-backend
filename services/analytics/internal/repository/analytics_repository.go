package repository

import (
	"context"

	"github.com/parkir-pintar/analytics/internal/model"
)

type AnalyticsRepository interface {
	Save(ctx context.Context, event *model.TransactionEvent) error
}
