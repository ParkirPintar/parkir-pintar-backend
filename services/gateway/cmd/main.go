package main

import (
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/parkir-pintar/gateway/internal/handler"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	searchConn := dial(envOr("SEARCH_ADDR", "localhost:50055"))
	defer searchConn.Close()

	reservationConn := dial(envOr("RESERVATION_ADDR", "localhost:50052"))
	defer reservationConn.Close()

	billingConn := dial(envOr("BILLING_ADDR", "localhost:50053"))
	defer billingConn.Close()

	paymentConn := dial(envOr("PAYMENT_ADDR", "localhost:50054"))
	defer paymentConn.Close()

	presenceConn := dial(envOr("PRESENCE_ADDR", "localhost:50056"))
	defer presenceConn.Close()

	h := handler.New(searchConn, reservationConn, billingConn, paymentConn, presenceConn)

	mux := http.NewServeMux()
	h.Register(mux)

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	addr := envOr("HTTP_ADDR", ":8080")

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Info().Msg("shutting down gateway")
		os.Exit(0)
	}()

	log.Info().Str("addr", addr).Msg("gateway starting")
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}

func dial(addr string) *grpc.ClientConn {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal().Err(err).Str("addr", addr).Msg("failed to dial")
	}
	log.Info().Str("addr", addr).Msg("connected")
	return conn
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
