package main

import (
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/parkir-pintar/presence/internal/adapter"
	"github.com/parkir-pintar/presence/internal/handler"
	"github.com/parkir-pintar/presence/internal/usecase"
	pb "github.com/parkir-pintar/presence/pkg/proto"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Reservation gRPC client
	reservationConn, err := grpc.NewClient(
		envOr("RESERVATION_ADDR", "localhost:50052"),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to Reservation Service")
	}
	defer reservationConn.Close()
	reservationClient := adapter.NewReservationClient(reservationConn)

	// Billing gRPC client
	billingConn, err := grpc.NewClient(
		envOr("BILLING_ADDR", "localhost:50053"),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to Billing Service")
	}
	defer billingConn.Close()
	billingClient := adapter.NewBillingClient(billingConn)

	// Usecase — Presence owns check-in trigger and billing start
	uc := usecase.NewPresenceUsecase(reservationClient, billingClient)

	// Handler
	h := handler.NewPresenceHandler(uc)

	// gRPC server
	addr := buildGRPCAddr(":50056")
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to listen")
	}

	srv := grpc.NewServer()
	pb.RegisterPresenceServiceServer(srv, h)
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(srv, healthSrv)
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	reflection.Register(srv)

	// HTTP health endpoint for K8s probes (port 8081)
	go func() {
		healthMux := http.NewServeMux()
		healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})
		log.Info().Msg("health endpoint listening on :8081")
		if err := http.ListenAndServe(":8081", healthMux); err != nil {
			log.Error().Err(err).Msg("health endpoint failed")
		}
	}()

	// HTTP REST API server (public-facing, port 8080)
	httpHandler := handler.NewHTTPHandler(uc)
	go func() {
		httpMux := http.NewServeMux()
		httpHandler.Register(httpMux)
		httpMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})
		httpAddr := envOr("HTTP_ADDR", ":8080")
		log.Info().Str("addr", httpAddr).Msg("HTTP REST API listening")
		if err := http.ListenAndServe(httpAddr, httpMux); err != nil {
			log.Fatal().Err(err).Msg("HTTP server failed")
		}
	}()

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Info().Msg("shutting down presence service")
		srv.GracefulStop()
	}()

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

func buildGRPCAddr(defaultAddr string) string {
	if v := os.Getenv("GRPC_ADDR"); v != "" {
		return v
	}
	if p := os.Getenv("GRPC_PORT"); p != "" {
		return ":" + p
	}
	return defaultAddr
}
