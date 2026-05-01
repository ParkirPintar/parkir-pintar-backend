package adapter

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
)

const eventsExchange = "events.exchange"

// EventPublisher abstracts publishing domain events to a message broker.
type EventPublisher interface {
	Publish(ctx context.Context, eventType string, payload []byte) error
}

type amqpPublisher struct {
	ch *amqp.Channel
}

// NewEventPublisher creates an EventPublisher backed by the given AMQP channel.
// It declares the events exchange on startup.
func NewEventPublisher(ch *amqp.Channel) EventPublisher {
	if err := ch.ExchangeDeclare(eventsExchange, "topic", true, false, false, false, nil); err != nil {
		log.Error().Err(err).Msg("failed to declare events exchange")
	}
	log.Info().Str("exchange", eventsExchange).Msg("exchange declared")

	return &amqpPublisher{ch: ch}
}

func (p *amqpPublisher) Publish(ctx context.Context, eventType string, payload []byte) error {
	err := p.ch.PublishWithContext(ctx,
		eventsExchange, // exchange
		eventType,      // routing key
		false,          // mandatory
		false,          // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent, // delivery mode 2
			Body:         payload,
		},
	)
	if err != nil {
		return fmt.Errorf("publish %s: %w", eventType, err)
	}

	log.Debug().
		Str("exchange", eventsExchange).
		Str("routing_key", eventType).
		Int("payload_size", len(payload)).
		Msg("event published")

	return nil
}
