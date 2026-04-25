package adapter

import (
	"context"
	"fmt"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

// publishCapture records the arguments of a publish call.
type publishCapture struct {
	exchange   string
	routingKey string
	msg        amqp.Publishing
}

// stubEventPublisher is a test double that captures publish calls.
type stubEventPublisher struct {
	bookingCaptures []publishCapture
	eventCaptures   []publishCapture
	bookingErr      error
	eventErr        error
}

func (s *stubEventPublisher) PublishBooking(_ context.Context, spotID string, payload []byte) error {
	if s.bookingErr != nil {
		return s.bookingErr
	}
	s.bookingCaptures = append(s.bookingCaptures, publishCapture{
		exchange:   bookingExchange,
		routingKey: spotID,
		msg: amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         payload,
		},
	})
	return nil
}

func (s *stubEventPublisher) PublishEvent(_ context.Context, eventType string, payload []byte) error {
	if s.eventErr != nil {
		return s.eventErr
	}
	s.eventCaptures = append(s.eventCaptures, publishCapture{
		exchange:   eventsExchange,
		routingKey: eventType,
		msg: amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         payload,
		},
	})
	return nil
}

func TestEventPublisher_Interface(t *testing.T) {
	// Verify that amqpPublisher satisfies EventPublisher at compile time.
	var _ EventPublisher = &amqpPublisher{}
}

func TestPublishBooking_Success(t *testing.T) {
	pub := &stubEventPublisher{}
	payload := []byte(`{"driver_id":"d-1","spot_id":"1-CAR-01","mode":"SYSTEM_ASSIGNED"}`)

	err := pub.PublishBooking(context.Background(), "1-CAR-01", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub.bookingCaptures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(pub.bookingCaptures))
	}

	cap := pub.bookingCaptures[0]
	if cap.exchange != "booking.exchange" {
		t.Errorf("exchange = %q, want %q", cap.exchange, "booking.exchange")
	}
	if cap.routingKey != "1-CAR-01" {
		t.Errorf("routingKey = %q, want %q", cap.routingKey, "1-CAR-01")
	}
	if cap.msg.ContentType != "application/json" {
		t.Errorf("ContentType = %q, want %q", cap.msg.ContentType, "application/json")
	}
	if cap.msg.DeliveryMode != amqp.Persistent {
		t.Errorf("DeliveryMode = %d, want %d", cap.msg.DeliveryMode, amqp.Persistent)
	}
	if string(cap.msg.Body) != string(payload) {
		t.Errorf("Body = %q, want %q", string(cap.msg.Body), string(payload))
	}
}

func TestPublishBooking_Error(t *testing.T) {
	pub := &stubEventPublisher{bookingErr: fmt.Errorf("channel closed")}

	err := pub.PublishBooking(context.Background(), "1-CAR-01", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPublishEvent_ReservationConfirmed(t *testing.T) {
	pub := &stubEventPublisher{}
	payload := []byte(`{"reservation_id":"res-1","spot_id":"1-CAR-01"}`)

	err := pub.PublishEvent(context.Background(), "reservation.confirmed", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub.eventCaptures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(pub.eventCaptures))
	}

	cap := pub.eventCaptures[0]
	if cap.exchange != "events.exchange" {
		t.Errorf("exchange = %q, want %q", cap.exchange, "events.exchange")
	}
	if cap.routingKey != "reservation.confirmed" {
		t.Errorf("routingKey = %q, want %q", cap.routingKey, "reservation.confirmed")
	}
}

func TestPublishEvent_CheckinConfirmed(t *testing.T) {
	pub := &stubEventPublisher{}
	payload := []byte(`{"reservation_id":"res-1"}`)

	err := pub.PublishEvent(context.Background(), "checkin.confirmed", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub.eventCaptures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(pub.eventCaptures))
	}
	if pub.eventCaptures[0].routingKey != "checkin.confirmed" {
		t.Errorf("routingKey = %q, want %q", pub.eventCaptures[0].routingKey, "checkin.confirmed")
	}
}

func TestPublishEvent_PenaltyApplied(t *testing.T) {
	pub := &stubEventPublisher{}
	payload := []byte(`{"reservation_id":"res-1","penalty":200000}`)

	err := pub.PublishEvent(context.Background(), "penalty.applied", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub.eventCaptures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(pub.eventCaptures))
	}
	if pub.eventCaptures[0].routingKey != "penalty.applied" {
		t.Errorf("routingKey = %q, want %q", pub.eventCaptures[0].routingKey, "penalty.applied")
	}
}

func TestPublishEvent_ReservationCancelled(t *testing.T) {
	pub := &stubEventPublisher{}

	err := pub.PublishEvent(context.Background(), "reservation.cancelled", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pub.eventCaptures[0].routingKey != "reservation.cancelled" {
		t.Errorf("routingKey = %q, want %q", pub.eventCaptures[0].routingKey, "reservation.cancelled")
	}
}

func TestPublishEvent_ReservationExpired(t *testing.T) {
	pub := &stubEventPublisher{}

	err := pub.PublishEvent(context.Background(), "reservation.expired", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pub.eventCaptures[0].routingKey != "reservation.expired" {
		t.Errorf("routingKey = %q, want %q", pub.eventCaptures[0].routingKey, "reservation.expired")
	}
}

func TestPublishEvent_Error(t *testing.T) {
	pub := &stubEventPublisher{eventErr: fmt.Errorf("channel closed")}

	err := pub.PublishEvent(context.Background(), "reservation.confirmed", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestNewEventPublisher_ReturnsEventPublisher(t *testing.T) {
	// NewEventPublisher accepts a nil channel for construction (will fail on Publish).
	// This test verifies the constructor returns the correct interface type.
	pub := NewEventPublisher(nil)
	if pub == nil {
		t.Fatal("expected non-nil EventPublisher")
	}
}
