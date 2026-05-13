package observability

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// DBSpanConfig holds configuration for database span creation.
type DBSpanConfig struct {
	// ServiceName is the logical name shown in service graph (e.g. "reservation-db", "redis")
	ServiceName string
	// DBSystem is the database system (e.g. "postgresql", "redis", "rabbitmq")
	DBSystem string
	// DBName is the database name (e.g. "reservation", "billing")
	DBName string
	// ServerAddress is the host:port of the database
	ServerAddress string
}

// StartDBSpan creates a span for a database operation with proper attributes
// for service graph topology (peer.service, db.system, db.name, server.address).
//
// Usage:
//
//	ctx, span := observability.StartDBSpan(ctx, "SELECT spots", observability.DBSpanConfig{
//	    ServiceName:   "reservation-db",
//	    DBSystem:      "postgresql",
//	    DBName:        "reservation",
//	    ServerAddress: "parkirpintar-reservation.xxx.rds.amazonaws.com:5432",
//	})
//	defer span.End()
func StartDBSpan(ctx context.Context, operation string, cfg DBSpanConfig) (context.Context, trace.Span) {
	tracer := otel.Tracer(fmt.Sprintf("db.%s", cfg.DBSystem))

	ctx, span := tracer.Start(ctx, operation,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("peer.service", cfg.ServiceName),
			attribute.String("db.system", cfg.DBSystem),
			attribute.String("db.name", cfg.DBName),
			attribute.String("server.address", cfg.ServerAddress),
			attribute.String("db.operation.name", operation),
		),
	)

	return ctx, span
}

// EndDBSpan ends a database span, recording error if present.
func EndDBSpan(span trace.Span, err error) {
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}

// PeerServiceFromDSN extracts a friendly peer.service name from a database connection string.
// Examples:
//   - "parkirpintar-reservation.xxx.rds.amazonaws.com:5432" -> "reservation-db"
//   - "parkirpintar.xxx.cache.amazonaws.com:6379" -> "redis"
//   - "b-xxx.mq.ap-southeast-1.on.aws:5671" -> "rabbitmq"
func PeerServiceFromDSN(host string, dbSystem string) string {
	switch dbSystem {
	case "postgresql":
		// Extract service name from RDS hostname: parkirpintar-{name}.xxx.rds.amazonaws.com
		parts := strings.Split(host, ".")
		if len(parts) > 0 {
			name := parts[0]
			// Remove "parkirpintar-" prefix
			name = strings.TrimPrefix(name, "parkirpintar-")
			if name == "parkirpintar" {
				return "redis" // fallback
			}
			return name + "-db"
		}
		return "postgresql"
	case "redis":
		return "redis"
	case "rabbitmq":
		return "rabbitmq"
	default:
		return dbSystem
	}
}
