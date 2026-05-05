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

	eventType := coalesce(strVal(raw, "event_type"), strVal(raw, "type"))
	if eventType == "" {
		// Skip events without identifiable type — cannot categorize
		return nil
	}

	event := &model.TransactionEvent{
		ID:            uuid.NewString(),
		EventType:     eventType,
		ReservationID: strVal(raw, "reservation_id"),
		DriverID:      strVal(raw, "driver_id"),
		SpotID:        strVal(raw, "spot_id"),
		VehicleType:   strVal(raw, "vehicle_type"),
		Payload:       body,
		RecordedAt:    time.Now(),
	}
	// Amount can come from "amount", "total", "booking_fee", or "cancellation_fee"
	if v, ok := raw["amount"].(float64); ok && v > 0 {
		event.Amount = int64(v)
	} else if v, ok := raw["total"].(float64); ok && v > 0 {
		event.Amount = int64(v)
	} else if v, ok := raw["booking_fee"].(float64); ok && v > 0 {
		event.Amount = int64(v)
	} else if v, ok := raw["cancellation_fee"].(float64); ok && v > 0 {
		event.Amount = int64(v)
	}
	return u.repo.Save(ctx, event)
}

func strVal(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
