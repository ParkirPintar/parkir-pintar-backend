package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parkir-pintar/billing/internal/adapter"
	"github.com/parkir-pintar/billing/internal/handler"
	"github.com/parkir-pintar/billing/internal/repository"
	"github.com/parkir-pintar/billing/internal/usecase"
	pb "github.com/parkir-pintar/billing/pkg/proto"
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

	ctx := context.Background()

	// --- Database (PostgreSQL) ---
	dbURL := envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/billing?sslmode=disable")
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer pool.Close()
	log.Info().Msg("connected to PostgreSQL")

	// --- Redis ---
	redisAddr := envOr("REDIS_ADDR", "localhost:6379")
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatal().Err(err).Msg("failed to connect to Redis")
	}
	defer rdb.Close()
	log.Info().Str("addr", redisAddr).Msg("connected to Redis")

	// --- Payment gRPC client ---
	paymentAddr := envOr("PAYMENT_ADDR", "localhost:50054")
	paymentConn, err := grpc.NewClient(paymentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal().Err(err).Str("addr", paymentAddr).Msg("failed to dial Payment service")
	}
	defer paymentConn.Close()
	paymentClient := adapter.NewPaymentClient(paymentConn)
	log.Info().Str("addr", paymentAddr).Msg("connected to Payment service")

	// --- RabbitMQ publisher ---
	rabbitURL := envOr("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	amqpConn, err := amqp.Dial(rabbitURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to RabbitMQ")
	}
	defer amqpConn.Close()
	amqpCh, err := amqpConn.Channel()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open AMQP channel")
	}
	defer amqpCh.Close()
	publisher := adapter.NewEventPublisher(amqpCh)
	log.Info().Msg("connected to RabbitMQ")

	// --- Wire dependencies ---
	repo := repository.NewBillingRepository(pool, rdb)
	uc := usecase.NewBillingUsecase(ctx, repo, paymentClient, publisher)
	h := handler.NewBillingHandler(uc)

	// --- Auth interceptor ---
	jwtSecret := envOr("JWT_SECRET", "parkir-pintar-secret")
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.UnaryAuthInterceptor(jwtSecret, rdb, nil)),
	)

	// --- Register gRPC service ---
	pb.RegisterBillingServiceServer(srv, h)
	reflection.Register(srv)

	// --- Start listener ---
	addr := envOr("GRPC_ADDR", ":50053")
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to listen")
	}

	// --- Graceful shutdown ---
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Info().Str("signal", sig.String()).Msg("shutting down gracefully")
		srv.GracefulStop()
	}()

	log.Info().Str("addr", addr).Msg("billing service starting")
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
