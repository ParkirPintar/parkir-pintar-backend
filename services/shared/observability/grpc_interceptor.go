package observability

import (
	"context"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// UnaryServerInterceptor returns a gRPC unary server interceptor that:
// - Extracts trace context from incoming metadata
// - Creates a server span for each RPC
// - Records gRPC status, method, and duration as span attributes
// - Sets span status on error
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	tracer := otel.Tracer("grpc.server")
	propagator := otel.GetTextMapPropagator()

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Extract trace context from gRPC metadata
		md, _ := metadata.FromIncomingContext(ctx)
		ctx = propagator.Extract(ctx, metadataCarrier(md))

		// Start span
		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.method", info.FullMethod),
			),
		)
		defer span.End()

		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		span.SetAttributes(attribute.Float64("rpc.duration_ms", float64(duration.Milliseconds())))

		if err != nil {
			st, _ := status.FromError(err)
			span.SetAttributes(attribute.String("rpc.grpc.status_code", st.Code().String()))
			span.SetStatus(codes.Error, st.Message())
			span.RecordError(err)
		} else {
			span.SetAttributes(attribute.String("rpc.grpc.status_code", "OK"))
			span.SetStatus(codes.Ok, "")
		}

		return resp, err
	}
}

// UnaryClientInterceptor returns a gRPC unary client interceptor that:
// - Injects trace context into outgoing metadata
// - Creates a client span for each outgoing RPC
// - Records gRPC status and duration
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	tracer := otel.Tracer("grpc.client")
	propagator := otel.GetTextMapPropagator()

	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// Derive peer.service from target address for service graph
		target := cc.Target()
		peerService := extractPeerService(target)

		// Start client span
		ctx, span := tracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.method", method),
				attribute.String("rpc.target", target),
				attribute.String("peer.service", peerService),
				attribute.String("server.address", target),
			),
		)
		defer span.End()

		// Inject trace context into outgoing metadata
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		} else {
			md = md.Copy()
		}
		propagator.Inject(ctx, metadataCarrier(md))
		ctx = metadata.NewOutgoingContext(ctx, md)

		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start)

		span.SetAttributes(attribute.Float64("rpc.duration_ms", float64(duration.Milliseconds())))

		if err != nil {
			st, _ := status.FromError(err)
			span.SetAttributes(attribute.String("rpc.grpc.status_code", st.Code().String()))
			span.SetStatus(codes.Error, st.Message())
			span.RecordError(err)
		} else {
			span.SetAttributes(attribute.String("rpc.grpc.status_code", "OK"))
			span.SetStatus(codes.Ok, "")
		}

		return err
	}
}

// metadataCarrier adapts gRPC metadata.MD to the OTel TextMapCarrier interface.
type metadataCarrier metadata.MD

func (mc metadataCarrier) Get(key string) string {
	vals := metadata.MD(mc).Get(key)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func (mc metadataCarrier) Set(key, value string) {
	metadata.MD(mc).Set(key, value)
}

func (mc metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(mc))
	for k := range mc {
		keys = append(keys, k)
	}
	return keys
}

// extractPeerService derives a human-readable service name from a gRPC target address.
// It handles formats like:
//   - "service-name.namespace.svc.cluster.local:50051" -> "service-name"
//   - "service-name:50051" -> "service-name"
//   - "10.0.1.2:50051" -> "10.0.1.2:50051" (unchanged for raw IPs)
func extractPeerService(target string) string {
	// Remove dns:/// prefix if present
	target = strings.TrimPrefix(target, "dns:///")

	// Split host:port
	host := target
	if idx := strings.LastIndex(target, ":"); idx > 0 {
		host = target[:idx]
	}

	// If it's a Kubernetes DNS name (contains dots), extract the service name
	if strings.Contains(host, ".") {
		parts := strings.Split(host, ".")
		// e.g. "billing-service.parkir-pintar.svc.cluster.local" -> "billing-service"
		return parts[0]
	}

	// If it's just a service name without dots, return as-is
	return host
}
