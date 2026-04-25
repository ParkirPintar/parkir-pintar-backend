package usecase

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/parkir-pintar/analytics/internal/model"
	"github.com/parkir-pintar/analytics/internal/repository"
)

type AnalyticsUsecase interface {
	RecordEvent(ctx context.Context, body []byte) error
}

type analyticsUsecase struct {
	repo repository.AnalyticsRepository
}

func NewAnalyticsUsecase(repo repository.AnalyticsRepository) AnalyticsUsecase {
	return &analyticsUsecase{repo: repo}
}

func (u *analyticsUsecase) RecordEvent(ctx context.Context, body []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	event := &model.TransactionEvent{
		ID:         uuid.NewString(),
		EventType:  strVal(raw, "type"),
		ReservationID: strVal(raw, "reservation_id"),
		SpotID:     strVal(raw, "spot_id"),
		Payload:    body,
		RecordedAt: time.Now(),
	}
	return u.repo.Save(ctx, event)
}

func strVal(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
