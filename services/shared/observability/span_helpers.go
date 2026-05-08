package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// StartSpan is a convenience wrapper that starts a new child span with the given
// name and optional attributes. Returns the updated context and span.
// Always defer span.End() after calling this.
//
// Usage:
//
//	ctx, span := observability.StartSpan(ctx, "reservation.usecase", "CreateReservation",
//	    attribute.String("driver_id", driverID),
//	    attribute.String("mode", mode),
//	)
//	defer span.End()
func StartSpan(ctx context.Context, tracerName, spanName string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	tracer := Tracer(tracerName)
	ctx, span := tracer.Start(ctx, spanName,
		trace.WithAttributes(attrs...),
	)
	return ctx, span
}

// RecordError records an error on the current span and sets the span status to Error.
// If err is nil, this is a no-op.
//
// Usage:
//
//	if err != nil {
//	    observability.RecordError(ctx, err, "failed to acquire lock")
//	    return err
//	}
func RecordError(ctx context.Context, err error, msg string) {
	if err == nil {
		return
	}
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.RecordError(err)
		span.SetStatus(codes.Error, fmt.Sprintf("%s: %v", msg, err))
	}
}

// SetSpanAttributes adds attributes to the current span in context.
// Useful for enriching spans with business data mid-execution.
//
// Usage:
//
//	observability.SetSpanAttributes(ctx,
//	    attribute.String("reservation_id", res.ID),
//	    attribute.Int64("booking_fee", res.BookingFee),
//	)
func SetSpanAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(attrs...)
	}
}

// SetSpanOk marks the current span as successful.
func SetSpanOk(ctx context.Context, msg string) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetStatus(codes.Ok, msg)
	}
}
