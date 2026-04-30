package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"os/signal"
	"strings"
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

func buildDatabaseURL(envKey, defaultDB string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	host := envOr("DB_HOST", "localhost")
	port := envOr("DB_PORT", "5432")
	user := envOr("DB_USER", "postgres")
	pass := envOr("DB_PASSWORD", "postgres")
	name := envOr("DB_NAME", defaultDB)
	sslmode := envOr("DB_SSLMODE", "disable")
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, name, sslmode)
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	ctx := context.Background()

	// PostgreSQL connection pool
	databaseURL := buildDatabaseURL("DATABASE_URL", "analytics")
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to PostgreSQL")
	}
	defer db.Close()
	log.Info().Msg("connected to PostgreSQL")

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
