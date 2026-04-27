package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parkir-pintar/reservation/internal/adapter"
	"github.com/parkir-pintar/reservation/internal/handler"
	"github.com/parkir-pintar/reservation/internal/repository"
	"github.com/parkir-pintar/reservation/internal/usecase"
	pb "github.com/parkir-pintar/reservation/pkg/proto"
	"github.com/parkir-pintar/user/pkg/interceptor"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Database
	dbPool, err := pgxpool.New(ctx, buildDatabaseURL("DATABASE_URL", "reservation"))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer dbPool.Close()

	// Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: buildRedisAddr(),
	})
	defer rdb.Close()

	// RabbitMQ
	amqpConn, err := amqp.Dial(envOr("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to RabbitMQ")
	}
	defer amqpConn.Close()

	amqpCh, err := amqpConn.Channel()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open AMQP channel")
	}
	defer amqpCh.Close()

	// gRPC client connections
	searchConn, err := grpc.NewClient(
		envOr("SEARCH_ADDR", "localhost:50055"),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to Search Service")
	}
	defer searchConn.Close()

	billingConn, err := grpc.NewClient(
		envOr("BILLING_ADDR", "localhost:50053"),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to Billing Service")
	}
	defer billingConn.Close()

	// Adapters
	searchClient := adapter.NewSearchClient(searchConn)
	billingClient := adapter.NewBillingClient(billingConn)
	publisher := adapter.NewEventPublisher(amqpCh)

	// Repository
	repo := repository.NewReservationRepository(dbPool, rdb)

	// Usecase
	uc := usecase.NewReservationUsecase(repo, searchClient, billingClient, publisher)

	// Handler
	h := handler.NewReservationHandler(uc)

	// gRPC server
	addr := buildGRPCAddr(":50052")
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to listen")
	}

	jwtSecret := envOr("JWT_SECRET", "parkir-pintar-secret")
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.UnaryAuthInterceptor(jwtSecret, rdb, nil)),
	)
	pb.RegisterReservationServiceServer(srv, h)
	reflection.Register(srv)

	// Start queue worker goroutine
	queueWorker := usecase.NewQueueWorker(repo, billingClient, searchClient, publisher, amqpCh, envOr("BOOKING_QUEUE", "booking.queue.0"))
	go func() {
		if err := queueWorker.Start(ctx); err != nil && ctx.Err() == nil {
			log.Error().Err(err).Msg("queue worker stopped unexpectedly")
		}
	}()

	// Start expiry worker goroutine
	expiryWorker := usecase.NewExpiryWorker(repo, billingClient, publisher)
	go func() {
		if err := expiryWorker.Start(ctx); err != nil && ctx.Err() == nil {
			log.Error().Err(err).Msg("expiry worker stopped unexpectedly")
		}
	}()

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Info().Msg("shutting down reservation service")
		cancel()
		srv.GracefulStop()
	}()

	log.Info().Str("addr", addr).Msg("reservation service starting")
	if err := srv.Serve(lis); err != nil {
		log.Fatal().Err(err).Msg("failed to serve")
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
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

func buildRedisAddr() string {
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		return v
	}
	host := envOr("REDIS_HOST", "localhost")
	port := envOr("REDIS_PORT", "6379")
	return host + ":" + port
}

func buildGRPCAddr(defaultAddr string) string {
	if v := os.Getenv("GRPC_ADDR"); v != "" {
		return v
	}
	if p := os.Getenv("GRPC_PORT"); p != "" {
		return ":" + p
	}
	return defaultAddr
}
