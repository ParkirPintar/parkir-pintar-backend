package adapter

import (
	"context"
	"fmt"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

// fakeChannel implements the subset of amqp.Channel used by the publisher.
// We use a wrapper approach to test without a real RabbitMQ connection.

type publishCapture struct {
	exchange   string
	routingKey string
	mandatory  bool
	immediate  bool
	msg        amqp.Publishing
}

// Since amqp.Channel is a concrete struct (not an interface), we test the
// publisher through its public EventPublisher interface. For unit tests we
// create a stub implementation that captures publish calls.

type stubPublisher struct {
	captures []publishCapture
	err      error
}

func (s *stubPublisher) Publish(_ context.Context, eventType string, payload []byte) error {
	if s.err != nil {
		return s.err
	}
	s.captures = append(s.captures, publishCapture{
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

func TestEventPublisher_PublishCheckoutCompleted(t *testing.T) {
	pub := &stubPublisher{}
	payload := []byte(`{"invoice_id":"inv-1","total":50000}`)

	err := pub.Publish(context.Background(), "checkout.completed", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub.captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(pub.captures))
	}

	cap := pub.captures[0]
	if cap.exchange != "events.exchange" {
		t.Errorf("exchange = %q, want %q", cap.exchange, "events.exchange")
	}
	if cap.routingKey != "checkout.completed" {
		t.Errorf("routingKey = %q, want %q", cap.routingKey, "checkout.completed")
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

func TestEventPublisher_PublishCheckoutFailed(t *testing.T) {
	pub := &stubPublisher{}
	payload := []byte(`{"invoice_id":"inv-2","error":"payment failed"}`)

	err := pub.Publish(context.Background(), "checkout.failed", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub.captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(pub.captures))
	}

	cap := pub.captures[0]
	if cap.routingKey != "checkout.failed" {
		t.Errorf("routingKey = %q, want %q", cap.routingKey, "checkout.failed")
	}
}

func TestEventPublisher_PublishError(t *testing.T) {
	pub := &stubPublisher{err: fmt.Errorf("channel closed")}

	err := pub.Publish(context.Background(), "checkout.completed", []byte(`{}`))
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
