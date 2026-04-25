package adapter

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
)

const (
	bookingExchange = "booking.exchange"
	eventsExchange  = "events.exchange"
)

// EventPublisher abstracts publishing messages to RabbitMQ exchanges.
type EventPublisher interface {
	// PublishBooking publishes a booking message to the consistent-hash
	// booking exchange with routing_key set to the spot ID.
	PublishBooking(ctx context.Context, spotID string, payload []byte) error

	// PublishEvent publishes a domain event to the topic events exchange
	// with routing_key set to the event type (e.g., "reservation.confirmed").
	PublishEvent(ctx context.Context, eventType string, payload []byte) error
}

type amqpPublisher struct {
	ch *amqp.Channel
}

// NewEventPublisher creates an EventPublisher backed by the given AMQP channel.
func NewEventPublisher(ch *amqp.Channel) EventPublisher {
	return &amqpPublisher{ch: ch}
}

func (p *amqpPublisher) PublishBooking(ctx context.Context, spotID string, payload []byte) error {
	err := p.ch.PublishWithContext(ctx,
		bookingExchange, // exchange (x-consistent-hash)
		spotID,          // routing key = spot_id for consistent hashing
		false,           // mandatory
		false,           // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         payload,
		},
	)
	if err != nil {
		return fmt.Errorf("publish booking to %s: %w", bookingExchange, err)
	}

	log.Debug().
		Str("exchange", bookingExchange).
		Str("routing_key", spotID).
		Int("payload_size", len(payload)).
		Msg("booking message published")

	return nil
}

func (p *amqpPublisher) PublishEvent(ctx context.Context, eventType string, payload []byte) error {
	err := p.ch.PublishWithContext(ctx,
		eventsExchange, // exchange (topic)
		eventType,      // routing key = event type
		false,          // mandatory
		false,          // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
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
