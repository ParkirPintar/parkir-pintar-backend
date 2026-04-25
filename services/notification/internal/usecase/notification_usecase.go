package usecase

import (
	"context"
	"encoding/json"

	"github.com/parkir-pintar/notification/internal/model"
	"github.com/rs/zerolog/log"
)

type NotificationUsecase interface {
	Handle(ctx context.Context, body []byte) error
}

// notifProvider is the interface for the external notification stub (push/SMS).
type notifProvider interface {
	Send(ctx context.Context, event model.NotificationEvent) error
}

type notificationUsecase struct {
	provider notifProvider
}

func NewNotificationUsecase(provider notifProvider) NotificationUsecase {
	return &notificationUsecase{provider: provider}
}

func (u *notificationUsecase) Handle(ctx context.Context, body []byte) error {
	var event model.NotificationEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return err
	}
	log.Info().Str("type", event.Type).Msg("notification event received")
	// gobreaker wrapping is done at the provider adapter level
	return u.provider.Send(ctx, event)
}
