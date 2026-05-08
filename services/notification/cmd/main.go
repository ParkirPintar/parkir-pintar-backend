package main

import (
	"context"
	"crypto/tls"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/parkir-pintar/shared/observability"
	amqp "github.com/rabbitmq/amqp091-go"
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
	observability.InitLogger(observability.LogConfig{
		ServiceName: "notification-service",
		Pretty:      os.Getenv("APP_ENV") == "local" || os.Getenv("APP_ENV") == "",
	})

	ctx := context.Background()

	shutdown, err := observability.InitTracer(ctx, observability.Config{
		ServiceName:    "notification-service",
		ServiceVersion: envOr("APP_VERSION", "dev"),
	})
	if err != nil {
		log.Warn().Err(err).Msg("failed to init tracer, continuing without tracing")
	} else {
		defer func() {
			if err := shutdown(ctx); err != nil {
				log.Error().Err(err).Msg("tracer shutdown error")
			}
		}()
	}

	// RabbitMQ connection
	rabbitmqURL := envOr("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	var conn *amqp.Connection
	if strings.HasPrefix(rabbitmqURL, "amqps://") {
		conn, err = amqp.DialTLS(rabbitmqURL, &tls.Config{})
	} else {
		conn, err = amqp.Dial(rabbitmqURL)
	}
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
