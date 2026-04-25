package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parkir-pintar/user/internal/handler"
	"github.com/parkir-pintar/user/internal/repository"
	"github.com/parkir-pintar/user/internal/usecase"
	pb "github.com/parkir-pintar/user/pkg/proto"
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
	dbURL := envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/user?sslmode=disable")
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

	// --- Wire dependencies ---
	repo := repository.NewUserRepository(pool, rdb)
	jwtHelper := usecase.NewJWTHelper()
	uc := usecase.NewUserUsecase(repo, jwtHelper)
	h := handler.NewUserHandler(uc)

	// --- Auth interceptor (skip Register and Login RPCs) ---
	jwtSecret := envOr("JWT_SECRET", "parkir-pintar-secret")
	skipMethods := []string{
		"/proto.UserService/Register",
		"/proto.UserService/Login",
	}
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.UnaryAuthInterceptor(jwtSecret, rdb, skipMethods)),
	)

	// --- Register gRPC service ---
	pb.RegisterUserServiceServer(srv, h)
	reflection.Register(srv)

	// --- Start listener ---
	addr := envOr("GRPC_ADDR", ":50051")
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

	log.Info().Str("addr", addr).Msg("user service starting")
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
