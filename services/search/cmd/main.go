package main

import (
	"net"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	addr := envOr("GRPC_ADDR", ":50051")
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to listen")
	}

	// TODO: init pgxpool, redis, wire dependencies
	// db, _ := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	// rdb := redis.NewClient(&redis.Options{Addr: os.Getenv("REDIS_ADDR")})
	// repo := repository.NewSpotRepository(db, rdb)
	// uc := usecase.NewSearchUsecase(repo)
	// h := handler.NewSearchHandler(uc)

	srv := grpc.NewServer()
	// pb.RegisterSearchServiceServer(srv, h)
	reflection.Register(srv)

	log.Info().Str("addr", addr).Msg("search service starting")
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
