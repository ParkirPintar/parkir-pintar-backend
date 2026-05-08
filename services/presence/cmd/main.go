package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/parkir-pintar/presence/internal/adapter"
	"github.com/parkir-pintar/presence/internal/handler"
	"github.com/parkir-pintar/presence/internal/usecase"
	pb "github.com/parkir-pintar/presence/pkg/proto"
	"github.com/parkir-pintar/shared/observability"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	observability.InitLogger(observability.LogConfig{
		ServiceName: "presence-service",
		Pretty:      os.Getenv("APP_ENV") == "local" || os.Getenv("APP_ENV") == "",
	})

	ctx := context.Background()

	shutdown, err := observability.InitTracer(ctx, observability.Config{
		ServiceName:    "presence-service",
		ServiceVersion: envOr("APP_VERSION", "dev"),
	})
	if err != nil {
		log.Warn().Err(err).Msg("failed to init tracer, continuing without tracing")
	} else {
		defer func() {
			if err := shutdown(ctx); err != nil {
				log.Error().Err(err).Msg("tracer shutdown error")
			}
		}()
	}

	// Reservation gRPC client
	reservationConn, err := grpc.NewClient(
		envOr("RESERVATION_ADDR", "localhost:50052"),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(observability.UnaryClientInterceptor()),
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
		grpc.WithUnaryInterceptor(observability.UnaryClientInterceptor()),
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

	srv := grpc.NewServer(
		grpc.UnaryInterceptor(observability.UnaryServerInterceptor()),
	)
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
		traced := observability.HTTPMiddleware("presence-service")(httpMux)
		log.Info().Str("addr", httpAddr).Msg("HTTP REST API listening")
		if err := http.ListenAndServe(httpAddr, traced); err != nil {
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
