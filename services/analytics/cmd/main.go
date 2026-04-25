package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/parkir-pintar/analytics/internal/handler"
	"github.com/parkir-pintar/analytics/internal/repository"
	"github.com/parkir-pintar/analytics/internal/usecase"
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

	ctx := context.Background()

	// PostgreSQL connection pool
	databaseURL := envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/analytics?sslmode=disable")
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to PostgreSQL")
	}
	defer db.Close()
	log.Info().Msg("connected to PostgreSQL")

	// RabbitMQ connection
	rabbitmqURL := envOr("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	conn, err := amqp.Dial(rabbitmqURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to RabbitMQ")
	}
	defer conn.Close()
	log.Info().Msg("connected to RabbitMQ")

	// Wire repo, usecase, and AMQP consumer handler
	repo := repository.NewAnalyticsRepository(db)
	uc := usecase.NewAnalyticsUsecase(repo)
	consumer := handler.NewAMQPConsumer(uc, conn)

	// Start consuming from analytics.queue
	if err := consumer.Start("analytics.queue"); err != nil {
		log.Fatal().Err(err).Msg("failed to start AMQP consumer")
	}
	log.Info().Msg("analytics service started, consuming from analytics.queue")

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("analytics service shutting down")
}
