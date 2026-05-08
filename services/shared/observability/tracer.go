// Package observability provides reusable OpenTelemetry instrumentation
// for all ParkirPintar microservices. It handles tracer provider setup,
// graceful shutdown, and OTLP export configuration.
package observability

import (
	"context"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Config holds the configuration for the tracer provider.
type Config struct {
	// ServiceName is the logical name of the service (e.g. "reservation-service").
	ServiceName string

	// ServiceVersion is the deployed version (e.g. git SHA or tag).
	ServiceVersion string

	// OTLPEndpoint is the gRPC endpoint of the OTel Collector.
	// Defaults to env OTEL_EXPORTER_OTLP_ENDPOINT or "localhost:4317".
	OTLPEndpoint string

	// Environment is the deployment environment (e.g. "production", "staging", "local").
	Environment string

	// SampleRatio controls the fraction of traces sampled (0.0 to 1.0).
	// Defaults to 1.0 (sample everything) if not set.
	SampleRatio float64
}

// Shutdown is a function that flushes and shuts down the tracer provider.
type Shutdown func(ctx context.Context) error

// InitTracer initializes the OpenTelemetry tracer provider with OTLP gRPC export.
// It returns a shutdown function that must be called on service exit.
//
// Usage:
//
//	shutdown, err := observability.InitTracer(ctx, observability.Config{
//	    ServiceName: "reservation-service",
//	    Environment: "production",
//	})
//	if err != nil { log.Fatal().Err(err).Msg("failed to init tracer") }
//	defer shutdown(ctx)
func InitTracer(ctx context.Context, cfg Config) (Shutdown, error) {
	if cfg.OTLPEndpoint == "" {
		cfg.OTLPEndpoint = envOr("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	}
	if cfg.Environment == "" {
		cfg.Environment = envOr("APP_ENV", "local")
	}
	if cfg.SampleRatio <= 0 {
		cfg.SampleRatio = 1.0
	}

	// Create OTLP gRPC exporter
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlptracegrpc.WithInsecure(), // mTLS handled by Istio sidecar
	)
	if err != nil {
		return nil, err
	}

	// Build resource with service metadata
	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", cfg.ServiceName),
			attribute.String("service.version", cfg.ServiceVersion),
			attribute.String("deployment.environment", cfg.Environment),
		),
	)
	if err != nil {
		return nil, err
	}

	// Create tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
	)

	// Register as global provider
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	shutdown := func(ctx context.Context) error {
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return tp.Shutdown(shutdownCtx)
	}

	return shutdown, nil
}

// Tracer returns a named tracer from the global provider.
// Use this in business logic to create custom spans.
//
// Usage:
//
//	tracer := observability.Tracer("reservation.usecase")
//	ctx, span := tracer.Start(ctx, "CreateReservation")
//	defer span.End()
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
