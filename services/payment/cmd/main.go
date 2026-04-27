package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parkir-pintar/payment/internal/adapter"
	"github.com/parkir-pintar/payment/internal/handler"
	"github.com/parkir-pintar/payment/internal/repository"
	"github.com/parkir-pintar/payment/internal/usecase"
	pb "github.com/parkir-pintar/payment/pkg/proto"
	"github.com/parkir-pintar/user/pkg/interceptor"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	ctx := context.Background()

	// --- Database (PostgreSQL) ---
	dbURL := buildDatabaseURL("DATABASE_URL", "payment")
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer pool.Close()
	log.Info().Msg("connected to PostgreSQL")

	// --- Redis ---
	redisAddr := buildRedisAddr()
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatal().Err(err).Msg("failed to connect to Redis")
	}
	defer rdb.Close()
	log.Info().Str("addr", redisAddr).Msg("connected to Redis")

	// --- Settlement HTTP client ---
	settlementURL := envOr("SETTLEMENT_URL", "http://localhost:8080")
	settlement := adapter.NewSettlementClient(settlementURL)
	log.Info().Str("url", settlementURL).Msg("settlement client configured")

	// --- Wire dependencies ---
	repo := repository.NewPaymentRepository(pool)
	uc := usecase.NewPaymentUsecase(repo, settlement)
	h := handler.NewPaymentHandler(uc)

	// --- Auth interceptor ---
	jwtSecret := envOr("JWT_SECRET", "parkir-pintar-secret")
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.UnaryAuthInterceptor(jwtSecret, rdb, nil)),
	)

	// --- Register gRPC service ---
	pb.RegisterPaymentServiceServer(srv, h)
	reflection.Register(srv)

	// --- Start listener ---
	addr := buildGRPCAddr(":50054")
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

	log.Info().Str("addr", addr).Msg("payment service starting")
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
