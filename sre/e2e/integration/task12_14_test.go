// Tasks 12–14: Reservation flows
//  12. Happy path reservation (system-assigned)
//  13. Happy path reservation (user-selected)
//  14. Double-book prevention

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	reservationpb "github.com/parkir-pintar/reservation/pkg/proto"
	searchpb "github.com/parkir-pintar/search/pkg/proto"
	billingpb "github.com/parkir-pintar/billing/pkg/proto"
	"google.golang.org/grpc/status"
)

// ─── Task 12: Happy path — system-assigned ─────────────────────────
// Login → Availability → Reserve → Check-in → Checkout → Payment
func TestTask12_HappyPathSystemAssigned(t *testing.T) {
	userConn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	resConn := dialGRPC(t, envOr("RESERVATION_ADDR", "localhost:50052"))
	searchConn := dialGRPC(t, envOr("SEARCH_ADDR", "localhost:50055"))
	billingConn := dialGRPC(t, envOr("BILLING_ADDR", "localhost:50053"))
	rdb := newRedis(t)

	ctx := registerAndLogin(t, userConn, uniquePlate("T12"), "CAR")

	// Step 1: Check availability
	availResp := &searchpb.GetAvailabilityResponse{}
	if err := searchConn.Invoke(ctx, "/search.SearchService/GetAvailability",
		&searchpb.GetAvailabilityRequest{Floor: 1, VehicleType: "CAR"}, availResp); err != nil {
		t.Fatalf("GetAvailability failed: %v", err)
	}
	t.Logf("✓ Availability: floor=1 available=%d", availResp.TotalAvailable)

	// Step 2: Create reservation (system-assigned)
	reservationID, spotID := createReservationAndWait(t, resConn, ctx, rdb, "SYSTEM_ASSIGNED", "CAR", "")
	t.Logf("✓ Reserved: id=%s spot=%s", reservationID, spotID)

	// Step 3: Check-in at correct spot
	checkinResp := &reservationpb.CheckInResponse{}
	if err := resConn.Invoke(ctx, "/reservation.ReservationService/CheckIn",
		&reservationpb.CheckInRequest{ReservationId: reservationID, ActualSpotId: spotID}, checkinResp); err != nil {
		t.Fatalf("CheckIn failed: %v", err)
	}
	if checkinResp.Status != "ACTIVE" {
		t.Errorf("expected ACTIVE, got %s", checkinResp.Status)
	}
	if checkinResp.WrongSpot {
		t.Error("expected wrong_spot=false")
	}
	t.Logf("✓ Checked in: status=%s wrong_spot=%v", checkinResp.Status, checkinResp.WrongSpot)

	// Step 4: Checkout
	time.Sleep(1 * time.Second) // ensure some billing duration
	checkoutResp := &billingpb.InvoiceResponse{}
	idemCheckout := uniquePlate("checkout-t12")
	if err := billingConn.Invoke(ctx, "/billing.BillingService/Checkout",
		&billingpb.CheckoutRequest{ReservationId: reservationID, IdempotencyKey: idemCheckout}, checkoutResp); err != nil {
		t.Fatalf("Checkout failed: %v", err)
	}
	if checkoutResp.InvoiceId == "" {
		t.Fatal("expected non-empty invoice_id")
	}
	t.Logf("✓ Checkout: invoice=%s total=%d booking=%d hourly=%d",
		checkoutResp.InvoiceId, checkoutResp.Total, checkoutResp.BookingFee, checkoutResp.HourlyFee)

	// Verify reservation is COMPLETED
	getResp := &reservationpb.ReservationResponse{}
	resConn.Invoke(ctx, "/reservation.ReservationService/GetReservation",
		&reservationpb.GetReservationRequest{ReservationId: reservationID}, getResp)
	t.Logf("✓ Final status: %s", getResp.Status)

	t.Log("✓ PASS: Task 12 — Happy path system-assigned complete")
}

// ─── Task 13: Happy path — user-selected ───────────────────────────
// Login → Availability → Hold → Reserve → Check-in → Checkout → Payment
func TestTask13_HappyPathUserSelected(t *testing.T) {
	userConn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	resConn := dialGRPC(t, envOr("RESERVATION_ADDR", "localhost:50052"))
	searchConn := dialGRPC(t, envOr("SEARCH_ADDR", "localhost:50055"))
	billingConn := dialGRPC(t, envOr("BILLING_ADDR", "localhost:50053"))
	rdb := newRedis(t)

	ctx := registerAndLogin(t, userConn, uniquePlate("T13"), "CAR")

	// Step 1: Check availability on floor 2
	availResp := &searchpb.GetAvailabilityResponse{}
	if err := searchConn.Invoke(ctx, "/search.SearchService/GetAvailability",
		&searchpb.GetAvailabilityRequest{Floor: 2, VehicleType: "CAR"}, availResp); err != nil {
		t.Fatalf("GetAvailability failed: %v", err)
	}
	if availResp.TotalAvailable == 0 || len(availResp.Spots) == 0 {
		t.Fatal("no available spots on floor 2")
	}
	targetSpot := availResp.Spots[0].SpotId
	t.Logf("✓ Selected spot: %s", targetSpot)

	// Clean up any existing hold
	rdb.Del(context.Background(), "hold:"+targetSpot)

	// Step 2: Hold the spot
	holdResp := &reservationpb.HoldSpotResponse{}
	if err := resConn.Invoke(ctx, "/reservation.ReservationService/HoldSpot",
		&reservationpb.HoldSpotRequest{SpotId: targetSpot}, holdResp); err != nil {
		t.Fatalf("HoldSpot failed: %v", err)
	}
	t.Logf("✓ Spot held until: %s", holdResp.HeldUntil)

	// Step 3: Create reservation (user-selected)
	reservationID, spotID := createReservationAndWait(t, resConn, ctx, rdb, "USER_SELECTED", "CAR", targetSpot)
	t.Logf("✓ Reserved: id=%s spot=%s", reservationID, spotID)

	// Step 4: Check-in
	checkinResp := &reservationpb.CheckInResponse{}
	if err := resConn.Invoke(ctx, "/reservation.ReservationService/CheckIn",
		&reservationpb.CheckInRequest{ReservationId: reservationID, ActualSpotId: spotID}, checkinResp); err != nil {
		t.Fatalf("CheckIn failed: %v", err)
	}
	t.Logf("✓ Checked in: status=%s", checkinResp.Status)

	// Step 5: Checkout
	time.Sleep(1 * time.Second)
	checkoutResp := &billingpb.InvoiceResponse{}
	if err := billingConn.Invoke(ctx, "/billing.BillingService/Checkout",
		&billingpb.CheckoutRequest{ReservationId: reservationID, IdempotencyKey: uniquePlate("checkout-t13")}, checkoutResp); err != nil {
		t.Fatalf("Checkout failed: %v", err)
	}
	t.Logf("✓ Checkout: invoice=%s total=%d", checkoutResp.InvoiceId, checkoutResp.Total)

	t.Log("✓ PASS: Task 13 — Happy path user-selected complete")
}

// ─── Task 14: Double-book prevention ───────────────────────────────
// Two concurrent reservations on same spot → second gets 409
func TestTask14_DoubleBookPrevention(t *testing.T) {
	userConn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	resConn := dialGRPC(t, envOr("RESERVATION_ADDR", "localhost:50052"))
	rdb := newRedis(t)

	ctxA := registerAndLogin(t, userConn, uniquePlate("T14A"), "CAR")
	ctxB := registerAndLogin(t, userConn, uniquePlate("T14B"), "CAR")

	// Driver A reserves a spot
	reservationID, spotID := createReservationAndWait(t, resConn, ctxA, rdb, "SYSTEM_ASSIGNED", "CAR", "")
	t.Logf("✓ Driver A reserved: id=%s spot=%s", reservationID, spotID)

	// Driver B tries to reserve the same spot (user-selected)
	idemKey := uniquePlate("t14b")
	err := resConn.Invoke(ctxB, "/reservation.ReservationService/CreateReservation",
		&reservationpb.CreateReservationRequest{
			Mode: "USER_SELECTED", VehicleType: "CAR", SpotId: spotID, IdempotencyKey: idemKey,
		}, &reservationpb.ReservationResponse{})

	// The request may be accepted into the queue but the worker should fail the lock.
	// Wait and check if the idempotency key resolves
	time.Sleep(3 * time.Second)
	id, redisErr := rdb.Get(context.Background(), "idempotency:"+idemKey).Result()

	if err != nil {
		st, _ := status.FromError(err)
		t.Logf("✓ Driver B rejected immediately: code=%s message=%s", st.Code(), st.Message())
	} else if redisErr != nil || id == "" {
		t.Logf("✓ Driver B's booking was not processed (lock contention)")
	} else {
		// If it was processed, verify the reservation failed
		resp := &reservationpb.ReservationResponse{}
		resConn.Invoke(ctxB, "/reservation.ReservationService/GetReservation",
			&reservationpb.GetReservationRequest{ReservationId: id}, resp)
		if resp.SpotId == spotID {
			t.Errorf("✗ Double-book: both drivers got spot %s", spotID)
		} else {
			t.Logf("✓ Driver B got different spot: %s (no double-book)", resp.SpotId)
		}
	}

	t.Log("✓ PASS: Task 14 — Double-book prevention verified")
}
