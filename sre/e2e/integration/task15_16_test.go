// Tasks 15 & 16 from the E2E test scenarios:
//   - Task 15: Spot contention / hold queue
//   - Task 16: Reservation expiry (no-show)

//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	reservationpb "github.com/parkir-pintar/reservation/pkg/proto"
	"google.golang.org/grpc/status"
)

// ─── Task 15: Spot contention / hold queue ─────────────────────────
// Driver A holds spot → Driver B tries same spot → 409 SPOT_HELD
func TestTask15_SpotContention(t *testing.T) {
	userConn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	resConn := dialGRPC(t, envOr("RESERVATION_ADDR", "localhost:50052"))
	rdb := newRedis(t)

	ctxA := registerAndLogin(t, userConn, uniquePlate("T15A"), "CAR")
	ctxB := registerAndLogin(t, userConn, uniquePlate("T15B"), "CAR")

	spotID := "1-CAR-01"
	rdb.Del(context.Background(), "hold:"+spotID)

	// Driver A holds the spot — should succeed
	holdResp := &reservationpb.HoldSpotResponse{}
	if err := resConn.Invoke(ctxA, "/reservation.ReservationService/HoldSpot",
		&reservationpb.HoldSpotRequest{SpotId: spotID}, holdResp); err != nil {
		t.Fatalf("Driver A hold failed: %v", err)
	}
	t.Logf("✓ Driver A hold OK: spot=%s held_until=%s", holdResp.SpotId, holdResp.HeldUntil)

	// Driver B tries same spot — should fail with AlreadyExists
	err := resConn.Invoke(ctxB, "/reservation.ReservationService/HoldSpot",
		&reservationpb.HoldSpotRequest{SpotId: spotID}, &reservationpb.HoldSpotResponse{})
	if err == nil {
		t.Fatal("✗ Driver B hold should have failed")
	}

	st, _ := status.FromError(err)
	t.Logf("✓ Driver B hold rejected: code=%s message=%s", st.Code(), st.Message())
	if st.Code() != 6 { // codes.AlreadyExists
		t.Errorf("expected AlreadyExists (6), got code=%d", st.Code())
	}

	rdb.Del(context.Background(), "hold:"+spotID)
	t.Log("✓ PASS: Task 15 — Spot contention correctly returns SPOT_HELD")
}

// ─── Task 16: Reservation expiry (no-show) ─────────────────────────
// Reserve → wait TTL → GET reservation → status=EXPIRED, spot released
func TestTask16_ReservationExpiry(t *testing.T) {
	userConn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	resConn := dialGRPC(t, envOr("RESERVATION_ADDR", "localhost:50052"))
	rdb := newRedis(t)
	db := connectDB(t, "DATABASE_URL", "postgres://parkir:parkir@localhost:5433/reservation_db?sslmode=disable")

	ctx := registerAndLogin(t, userConn, uniquePlate("T16"), "CAR")

	reservationID, spotID := createReservationAndWait(t, resConn, ctx, rdb, "SYSTEM_ASSIGNED", "CAR", "")
	t.Logf("✓ Reservation created: id=%s spot=%s", reservationID, spotID)

	// Verify RESERVED status
	getResp := &reservationpb.ReservationResponse{}
	if err := resConn.Invoke(ctx, "/reservation.ReservationService/GetReservation",
		&reservationpb.GetReservationRequest{ReservationId: reservationID}, getResp); err != nil {
		t.Fatalf("GetReservation failed: %v", err)
	}
	if getResp.Status != "RESERVED" {
		t.Fatalf("expected RESERVED, got %s", getResp.Status)
	}

	// Simulate expiry by setting expires_at to the past
	_, err := db.Exec(context.Background(),
		"UPDATE reservations SET expires_at = now() - interval '1 minute' WHERE id = $1", reservationID)
	if err != nil {
		t.Fatalf("update expires_at: %v", err)
	}
	t.Log("✓ expires_at set to past (simulating no-show)")

	// Wait for expiry worker
	t.Log("  Waiting for expiry worker (up to 45s)...")
	var finalStatus string
	for i := 0; i < 15; i++ {
		time.Sleep(3 * time.Second)
		resp := &reservationpb.ReservationResponse{}
		if err := resConn.Invoke(ctx, "/reservation.ReservationService/GetReservation",
			&reservationpb.GetReservationRequest{ReservationId: reservationID}, resp); err != nil {
			continue
		}
		t.Logf("  [%ds] status=%s", (i+1)*3, resp.Status)
		if resp.Status == "EXPIRED" {
			finalStatus = "EXPIRED"
			break
		}
	}
	if finalStatus != "EXPIRED" {
		t.Fatalf("expected EXPIRED, got %s", finalStatus)
	}

	// Verify Redis lock released
	_, err = rdb.Get(context.Background(), "lock:"+spotID).Result()
	if err != nil {
		t.Logf("✓ Lock released for spot %s", spotID)
	} else {
		t.Error("✗ Lock should have been released after expiry")
	}

	t.Log(fmt.Sprintf("✓ PASS: Task 16 — Reservation expired, spot %s released", spotID))
}
