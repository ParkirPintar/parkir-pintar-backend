// Tasks 24–25: Idempotency
//  24. Idempotency — duplicate reservation
//  25. Idempotency — duplicate checkout

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	reservationpb "github.com/parkir-pintar/reservation/pkg/proto"
	billingpb "github.com/parkir-pintar/billing/pkg/proto"
)

// ─── Task 24: Idempotency — duplicate reservation ──────────────────
// Same Idempotency-Key twice → same reservation_id returned
func TestTask24_IdempotentReservation(t *testing.T) {
	userConn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	resConn := dialGRPC(t, envOr("RESERVATION_ADDR", "localhost:50052"))
	rdb := newRedis(t)

	ctx := registerAndLogin(t, userConn, uniquePlate("T24"), "CAR")

	idemKey := uniquePlate("idem-t24")

	// First request
	req := &reservationpb.CreateReservationRequest{
		Mode: "SYSTEM_ASSIGNED", VehicleType: "CAR", IdempotencyKey: idemKey,
	}
	resp1 := &reservationpb.ReservationResponse{}
	if err := resConn.Invoke(ctx, "/reservation.ReservationService/CreateReservation", req, resp1); err != nil {
		t.Fatalf("First CreateReservation failed: %v", err)
	}

	// Wait for queue worker
	var reservationID string
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)
		id, err := rdb.Get(context.Background(), "idempotency:"+idemKey).Result()
		if err == nil && id != "" {
			reservationID = id
			break
		}
	}
	if reservationID == "" {
		t.Fatal("Queue worker did not process first request")
	}
	t.Logf("✓ First request: reservation_id=%s", reservationID)

	// Second request with same idempotency key
	resp2 := &reservationpb.ReservationResponse{}
	if err := resConn.Invoke(ctx, "/reservation.ReservationService/CreateReservation", req, resp2); err != nil {
		t.Fatalf("Second CreateReservation failed: %v", err)
	}

	// The second call should return the same reservation (from idempotency cache)
	if resp2.ReservationId != "" && resp2.ReservationId != reservationID {
		t.Errorf("expected same reservation_id=%s, got %s", reservationID, resp2.ReservationId)
	}

	// Also verify via Redis
	id2, _ := rdb.Get(context.Background(), "idempotency:"+idemKey).Result()
	if id2 != reservationID {
		t.Errorf("idempotency key should still map to %s, got %s", reservationID, id2)
	}
	t.Logf("✓ Second request: same reservation_id=%s (idempotent)", reservationID)

	t.Log("✓ PASS: Task 24 — Duplicate reservation returns same ID")
}

// ─── Task 25: Idempotency — duplicate checkout ─────────────────────
// Same Idempotency-Key twice → same invoice_id returned
func TestTask25_IdempotentCheckout(t *testing.T) {
	userConn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	resConn := dialGRPC(t, envOr("RESERVATION_ADDR", "localhost:50052"))
	billingConn := dialGRPC(t, envOr("BILLING_ADDR", "localhost:50053"))
	rdb := newRedis(t)

	ctx := registerAndLogin(t, userConn, uniquePlate("T25"), "CAR")

	// Reserve + check-in
	reservationID, spotID := createReservationAndWait(t, resConn, ctx, rdb, "SYSTEM_ASSIGNED", "CAR", "")
	resConn.Invoke(ctx, "/reservation.ReservationService/CheckIn",
		&reservationpb.CheckInRequest{ReservationId: reservationID, ActualSpotId: spotID},
		&reservationpb.CheckInResponse{})

	time.Sleep(1 * time.Second)

	idemKey := uniquePlate("checkout-idem-t25")

	// First checkout
	resp1 := &billingpb.InvoiceResponse{}
	if err := billingConn.Invoke(ctx, "/billing.BillingService/Checkout",
		&billingpb.CheckoutRequest{ReservationId: reservationID, IdempotencyKey: idemKey}, resp1); err != nil {
		t.Fatalf("First Checkout failed: %v", err)
	}
	t.Logf("✓ First checkout: invoice_id=%s total=%d", resp1.InvoiceId, resp1.Total)

	// Second checkout with same idempotency key
	resp2 := &billingpb.InvoiceResponse{}
	if err := billingConn.Invoke(ctx, "/billing.BillingService/Checkout",
		&billingpb.CheckoutRequest{ReservationId: reservationID, IdempotencyKey: idemKey}, resp2); err != nil {
		t.Fatalf("Second Checkout failed: %v", err)
	}

	if resp2.InvoiceId != resp1.InvoiceId {
		t.Errorf("expected same invoice_id=%s, got %s", resp1.InvoiceId, resp2.InvoiceId)
	}
	if resp2.Total != resp1.Total {
		t.Errorf("expected same total=%d, got %d", resp1.Total, resp2.Total)
	}
	t.Logf("✓ Second checkout: invoice_id=%s (idempotent)", resp2.InvoiceId)

	t.Log("✓ PASS: Task 25 — Duplicate checkout returns same invoice")
}
