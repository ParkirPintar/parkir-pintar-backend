package observability

import (
	"context"
	"net"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// RedisTracingHook implements redis.Hook to add OpenTelemetry spans
// with peer.service attribute for service graph topology.
type RedisTracingHook struct {
	tracer trace.Tracer
	addr   string
}

// NewRedisTracingHook creates a new Redis tracing hook.
// The addr parameter is the Redis server address (e.g. "redis-host:6379").
func NewRedisTracingHook(addr string) *RedisTracingHook {
	return &RedisTracingHook{
		tracer: otel.Tracer("db.redis"),
		addr:   addr,
	}
}

func (h *RedisTracingHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (h *RedisTracingHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		ctx, span := h.tracer.Start(ctx, cmd.Name(),
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String("peer.service", "redis"),
				attribute.String("db.system", "redis"),
				attribute.String("db.operation.name", cmd.Name()),
				attribute.String("server.address", h.addr),
			),
		)
		defer span.End()

		err := next(ctx, cmd)
		if err != nil && err != redis.Nil {
			span.SetStatus(codes.Error, err.Error())
			span.RecordError(err)
		} else {
			span.SetStatus(codes.Ok, "")
		}
		return err
	}
}

func (h *RedisTracingHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		ctx, span := h.tracer.Start(ctx, "pipeline",
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String("peer.service", "redis"),
				attribute.String("db.system", "redis"),
				attribute.String("db.operation.name", "pipeline"),
				attribute.String("server.address", h.addr),
				attribute.Int("db.redis.pipeline_length", len(cmds)),
			),
		)
		defer span.End()

		err := next(ctx, cmds)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.RecordError(err)
		} else {
			span.SetStatus(codes.Ok, "")
		}
		return err
	}
}
