package observability

import (
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// HTTPMiddleware returns an http.Handler that wraps the given handler with
// OpenTelemetry tracing. It extracts incoming trace context, creates a server
// span, and records HTTP method, route, status code, and duration.
//
// Usage:
//
//	mux := http.NewServeMux()
//	mux.HandleFunc("/v1/reservations", handler)
//	traced := observability.HTTPMiddleware("reservation-service")(mux)
//	http.ListenAndServe(":8080", traced)
func HTTPMiddleware(serviceName string) func(http.Handler) http.Handler {
	tracer := otel.Tracer(fmt.Sprintf("%s.http", serviceName))
	propagator := otel.GetTextMapPropagator()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract trace context from HTTP headers
			ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			spanName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
			ctx, span := tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					semconv.HTTPRequestMethodKey.String(r.Method),
					semconv.URLPath(r.URL.Path),
					semconv.ServerAddress(r.Host),
					semconv.UserAgentOriginal(r.UserAgent()),
				),
			)
			defer span.End()

			// Wrap response writer to capture status code
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			start := time.Now()
			next.ServeHTTP(rw, r.WithContext(ctx))
			duration := time.Since(start)

			span.SetAttributes(
				semconv.HTTPResponseStatusCode(rw.statusCode),
				attribute.Float64("http.duration_ms", float64(duration.Milliseconds())),
			)

			if rw.statusCode >= 400 {
				span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", rw.statusCode))
			} else {
				span.SetStatus(codes.Ok, "")
			}
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
