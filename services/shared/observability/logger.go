package observability

import (
	"context"
	"io"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/trace"
)

// LogConfig holds configuration for the structured logger.
type LogConfig struct {
	// ServiceName is added to every log entry as "service" field.
	ServiceName string

	// Level sets the minimum log level. Defaults to "info".
	// Valid values: "trace", "debug", "info", "warn", "error", "fatal".
	Level string

	// Pretty enables human-readable console output (for local dev).
	// In production, set to false for JSON output.
	Pretty bool
}

// InitLogger configures the global zerolog logger with service metadata
// and optional pretty-printing for development.
//
// Usage:
//
//	observability.InitLogger(observability.LogConfig{
//	    ServiceName: "reservation-service",
//	    Pretty:      os.Getenv("APP_ENV") == "local",
//	})
func InitLogger(cfg LogConfig) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	level := parseLevel(cfg.Level)
	zerolog.SetGlobalLevel(level)

	var writer io.Writer = os.Stderr
	if cfg.Pretty {
		writer = zerolog.ConsoleWriter{Out: os.Stderr}
	}

	log.Logger = zerolog.New(writer).
		With().
		Timestamp().
		Str("service", cfg.ServiceName).
		Logger()
}

// LoggerFromContext returns a zerolog.Logger enriched with trace_id and span_id
// from the current span in context. This correlates logs with traces in Grafana.
//
// Usage:
//
//	logger := observability.LoggerFromContext(ctx)
//	logger.Info().Str("driver_id", driverID).Msg("reservation created")
func LoggerFromContext(ctx context.Context) *zerolog.Logger {
	span := trace.SpanFromContext(ctx)
	sc := span.SpanContext()

	logger := log.Logger.With().Logger()

	if sc.HasTraceID() {
		logger = logger.With().
			Str("trace_id", sc.TraceID().String()).
			Str("span_id", sc.SpanID().String()).
			Logger()
	}

	return &logger
}

// SpanEvent adds a zerolog-style event as a span event for trace-level debugging.
// This is useful for recording business events directly on the trace.
//
// Usage:
//
//	observability.SpanEvent(ctx, "lock_acquired", attribute.String("spot_id", spotID))
func SpanEvent(ctx context.Context, name string, attrs ...trace.EventOption) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.AddEvent(name, attrs...)
	}
}

func parseLevel(s string) zerolog.Level {
	switch s {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	default:
		return zerolog.InfoLevel
	}
}
