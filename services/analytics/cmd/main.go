package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// TODO: init pgxpool, RabbitMQ connection, wire dependencies
	// db, _ := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	// conn, _ := amqp.Dial(os.Getenv("RABBITMQ_URL"))
	// repo := repository.NewAnalyticsRepository(db)
	// uc := usecase.NewAnalyticsUsecase(repo)
	// consumer := handler.NewAMQPConsumer(uc, conn)
	// consumer.Start("analytics.queue")

	log.Info().Msg("analytics service starting")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("analytics service shutting down")
}
