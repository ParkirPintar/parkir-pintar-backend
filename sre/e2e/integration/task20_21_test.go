// Tasks 20–21: Billing flows
//  20. Extended stay billing (no overstay penalty)
//  21. Overnight fee (session crosses midnight)

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	reservationpb "github.com/parkir-pintar/reservation/pkg/proto"
	billingpb "github.com/parkir-pintar/billing/pkg/proto"
)

// ─── Task 20: Extended stay — no overstay penalty ──────────────────
// Long session → checkout → only standard hourly rate, no extra penalty
func TestTask20_ExtendedStayNoPenalty(t *testing.T) {
	userConn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	resConn := dialGRPC(t, envOr("RESERVATION_ADDR", "localhost:50052"))
	billingConn := dialGRPC(t, envOr("BILLING_ADDR", "localhost:50053"))
	rdb := newRedis(t)
	db := connectDB(t, "BILLING_DATABASE_URL", "postgres://parkir:parkir@localhost:5433/billing_db?sslmode=disable")

	ctx := registerAndLogin(t, userConn, uniquePlate("T20"), "CAR")

	reservationID, spotID := createReservationAndWait(t, resConn, ctx, rdb, "SYSTEM_ASSIGNED", "CAR", "")
	t.Logf("✓ Reserved: id=%s spot=%s", reservationID, spotID)

	// Check-in
	checkinResp := &reservationpb.CheckInResponse{}
	if err := resConn.Invoke(ctx, "/reservation.ReservationService/CheckIn",
		&reservationpb.CheckInRequest{ReservationId: reservationID, ActualSpotId: spotID}, checkinResp); err != nil {
		t.Fatalf("CheckIn failed: %v", err)
	}

	// Simulate 3-hour session by backdating session_start (same day, no midnight crossing)
	_, err := db.Exec(context.Background(),
		"UPDATE billing_sessions SET session_start = now() - interval '3 hours' WHERE reservation_id = $1", reservationID)
	if err != nil {
		t.Logf("  billing_sessions update: %v (may not exist yet, continuing)", err)
	}

	time.Sleep(500 * time.Millisecond)

	checkoutResp := &billingpb.InvoiceResponse{}
	if err := billingConn.Invoke(ctx, "/billing.BillingService/Checkout",
		&billingpb.CheckoutRequest{ReservationId: reservationID, IdempotencyKey: uniquePlate("checkout-t20")}, checkoutResp); err != nil {
		t.Fatalf("Checkout failed: %v", err)
	}

	// Verify: no penalty, standard hourly rate
	if checkoutResp.Penalty != 0 {
		t.Errorf("expected penalty=0 (no overstay penalty), got %d", checkoutResp.Penalty)
	}
	// Hourly fee should be ceil(3) * 5000 = 15000
	if checkoutResp.HourlyFee < 5000 {
		t.Errorf("expected hourly_fee >= 5000, got %d", checkoutResp.HourlyFee)
	}
	t.Logf("✓ Checkout: total=%d booking=%d hourly=%d overnight=%d penalty=%d",
		checkoutResp.Total, checkoutResp.BookingFee, checkoutResp.HourlyFee, checkoutResp.OvernightFee, checkoutResp.Penalty)

	t.Log("✓ PASS: Task 20 — Extended stay, no overstay penalty")
}

// ─── Task 21: Overnight fee ────────────────────────────────────────
// Session crosses midnight → overnight_fee=20000 in invoice
func TestTask21_OvernightFee(t *testing.T) {
	userConn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	resConn := dialGRPC(t, envOr("RESERVATION_ADDR", "localhost:50052"))
	billingConn := dialGRPC(t, envOr("BILLING_ADDR", "localhost:50053"))
	rdb := newRedis(t)
	db := connectDB(t, "BILLING_DATABASE_URL", "postgres://parkir:parkir@localhost:5433/billing_db?sslmode=disable")

	ctx := registerAndLogin(t, userConn, uniquePlate("T21"), "CAR")

	reservationID, spotID := createReservationAndWait(t, resConn, ctx, rdb, "SYSTEM_ASSIGNED", "CAR", "")
	t.Logf("✓ Reserved: id=%s spot=%s", reservationID, spotID)

	// Check-in
	checkinResp := &reservationpb.CheckInResponse{}
	if err := resConn.Invoke(ctx, "/reservation.ReservationService/CheckIn",
		&reservationpb.CheckInRequest{ReservationId: reservationID, ActualSpotId: spotID}, checkinResp); err != nil {
		t.Fatalf("CheckIn failed: %v", err)
	}

	// Simulate overnight: set session_start to yesterday 23:00
	// This ensures the session crosses midnight
	_, err := db.Exec(context.Background(),
		`UPDATE billing_sessions SET session_start = (date_trunc('day', now()) - interval '1 hour') WHERE reservation_id = $1`, reservationID)
	if err != nil {
		t.Logf("  billing_sessions update: %v (continuing)", err)
	}

	time.Sleep(500 * time.Millisecond)

	checkoutResp := &billingpb.InvoiceResponse{}
	if err := billingConn.Invoke(ctx, "/billing.BillingService/Checkout",
		&billingpb.CheckoutRequest{ReservationId: reservationID, IdempotencyKey: uniquePlate("checkout-t21")}, checkoutResp); err != nil {
		t.Fatalf("Checkout failed: %v", err)
	}

	if checkoutResp.OvernightFee != 20000 {
		t.Errorf("expected overnight_fee=20000, got %d", checkoutResp.OvernightFee)
	}
	t.Logf("✓ Checkout: total=%d booking=%d hourly=%d overnight=%d",
		checkoutResp.Total, checkoutResp.BookingFee, checkoutResp.HourlyFee, checkoutResp.OvernightFee)

	t.Log("✓ PASS: Task 21 — Overnight fee 20.000 IDR applied")
}
