package main

import (
	"os"
	"os/signal"
	"syscall"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/parkir-pintar/notification/internal/adapter"
	"github.com/parkir-pintar/notification/internal/handler"
	"github.com/parkir-pintar/notification/internal/usecase"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// RabbitMQ connection
	rabbitmqURL := envOr("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	conn, err := amqp.Dial(rabbitmqURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to RabbitMQ")
	}
	defer conn.Close()
	log.Info().Msg("connected to RabbitMQ")

	// Notification provider stub client (HTTP + gobreaker circuit breaker)
	notifURL := envOr("NOTIF_PROVIDER_URL", "http://localhost:9090")
	provider := adapter.NewNotifProvider(notifURL)

	// Wire usecase and AMQP consumer handler
	uc := usecase.NewNotificationUsecase(provider)
	consumer := handler.NewAMQPConsumer(uc, conn)

	// Start consuming from notification.queue
	if err := consumer.Start("notification.queue"); err != nil {
		log.Fatal().Err(err).Msg("failed to start AMQP consumer")
	}
	log.Info().Msg("notification service started, consuming from notification.queue")

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("notification service shutting down")
}
