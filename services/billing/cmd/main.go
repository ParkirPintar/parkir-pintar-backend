package main

import (
	"context"
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

	ctx := context.Background()
	addr := envOr("GRPC_ADDR", ":50053")
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to listen")
	}

	// TODO: init pgxpool, wire dependencies
	// db, _ := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	// repo := repository.NewBillingRepository(db)
	// uc := usecase.NewBillingUsecase(ctx, repo)
	// h := handler.NewBillingHandler(uc)
	_ = ctx

	srv := grpc.NewServer()
	// pb.RegisterBillingServiceServer(srv, h)
	reflection.Register(srv)

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
