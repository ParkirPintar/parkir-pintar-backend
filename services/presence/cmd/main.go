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

	addr := envOr("GRPC_ADDR", ":50055")
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to listen")
	}

	// TODO: init reservation gRPC client, wire dependencies
	// reservationConn, _ := grpc.Dial(os.Getenv("RESERVATION_ADDR"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	// reservationClient := adapter.NewReservationClient(reservationConn)
	// uc := usecase.NewPresenceUsecase(reservationClient)
	// h := handler.NewPresenceHandler(uc)

	srv := grpc.NewServer()
	// pb.RegisterPresenceServiceServer(srv, h)
	reflection.Register(srv)

	log.Info().Str("addr", addr).Msg("presence service starting")
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
