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

	// TODO: init RabbitMQ connection, notification provider stub, wire dependencies
	// conn, _ := amqp.Dial(os.Getenv("RABBITMQ_URL"))
	// provider := stub.NewNotificationProvider(os.Getenv("NOTIF_PROVIDER_URL"))
	// uc := usecase.NewNotificationUsecase(provider)
	// consumer := handler.NewAMQPConsumer(uc, conn)
	// consumer.Start("notification.queue")

	log.Info().Msg("notification service starting")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("notification service shutting down")
}
