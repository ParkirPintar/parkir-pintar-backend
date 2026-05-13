package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Cyprinus12138/otelgin"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parkir-pintar/billing/internal/adapter"
	"github.com/parkir-pintar/billing/internal/handler"
	"github.com/parkir-pintar/billing/internal/repository"
	"github.com/parkir-pintar/billing/internal/usecase"
	pb "github.com/parkir-pintar/billing/pkg/proto"
	"github.com/parkir-pintar/shared/observability"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	observability.InitLogger(observability.LogConfig{
		ServiceName: "billing-service",
		Pretty:      os.Getenv("APP_ENV") == "local" || os.Getenv("APP_ENV") == "",
	})

	ctx := context.Background()

	shutdown, err := observability.InitTracer(ctx, observability.Config{
		ServiceName:    "billing-service",
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

	// --- Database (PostgreSQL) ---
	dbURL := buildDatabaseURL("DATABASE_URL", "billing")
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

	// --- Payment gRPC client ---
	paymentAddr := envOr("PAYMENT_ADDR", "localhost:50054")
	paymentConn, err := grpc.NewClient(paymentAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(observability.UnaryClientInterceptor()),
	)
	if err != nil {
		log.Fatal().Err(err).Str("addr", paymentAddr).Msg("failed to dial Payment service")
	}
	defer paymentConn.Close()
	paymentClient := adapter.NewPaymentClient(paymentConn)
	log.Info().Str("addr", paymentAddr).Msg("connected to Payment service")

	// --- RabbitMQ publisher ---
	rabbitURL := envOr("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	var amqpConn *amqp.Connection
	if strings.HasPrefix(rabbitURL, "amqps://") {
		amqpConn, err = amqp.DialTLS(rabbitURL, &tls.Config{})
	} else {
		amqpConn, err = amqp.Dial(rabbitURL)
	}
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
	uc, err := usecase.NewBillingUsecase(ctx, repo, paymentClient, publisher)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize billing usecase — gorules pricing engine required")
	}
	h := handler.NewBillingHandler(uc)

	// --- gRPC server ---
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(observability.UnaryServerInterceptor()),
	)

	// --- Register gRPC service ---
	pb.RegisterBillingServiceServer(srv, h)
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

	// HTTP REST API server (public-facing, port 8080) with Gin + otelgin tracing
	httpHandler := handler.NewHTTPHandler(uc)
	go func() {
		gin.SetMode(gin.ReleaseMode)
		r := gin.New()
		r.Use(gin.Recovery())
		r.Use(otelgin.Middleware("billing-service"))
		httpHandler.Register(r)
		r.GET("/healthz", func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})
		httpAddr := envOr("HTTP_ADDR", ":8080")
		log.Info().Str("addr", httpAddr).Msg("HTTP REST API listening")
		if err := r.Run(httpAddr); err != nil {
			log.Fatal().Err(err).Msg("HTTP server failed")
		}
	}()

	// --- Start listener ---
	addr := buildGRPCAddr(":50053")
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
