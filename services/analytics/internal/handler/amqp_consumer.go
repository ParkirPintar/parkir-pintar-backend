package handler

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/parkir-pintar/analytics/internal/usecase"
	"github.com/rs/zerolog/log"
)

type AMQPConsumer struct {
	uc   usecase.AnalyticsUsecase
	conn *amqp.Connection
}

func NewAMQPConsumer(uc usecase.AnalyticsUsecase, conn *amqp.Connection) *AMQPConsumer {
	return &AMQPConsumer{uc: uc, conn: conn}
}

func (c *AMQPConsumer) Start(queue string) error {
	ch, err := c.conn.Channel()
	if err != nil {
		return err
	}

	msgs, err := ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	go func() {
		for msg := range msgs {
			ctx := context.Background()
			if err := c.uc.RecordEvent(ctx, msg.Body); err != nil {
				log.Error().Err(err).Msg("analytics record error")
				_ = msg.Nack(false, true)
				continue
			}
			_ = msg.Ack(false)
		}
	}()
	return nil
}
