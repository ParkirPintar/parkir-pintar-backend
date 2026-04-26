// Tasks 18–19: Cancellation flows
//  18. Cancellation ≤ 2 min (free)
//  19. Cancellation > 2 min (5.000 IDR)

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	reservationpb "github.com/parkir-pintar/reservation/pkg/proto"
)

func TestTask18_CancelFree(t *testing.T) {
	userConn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	resConn := dialGRPC(t, envOr("RESERVATION_ADDR", "localhost:50052"))
	rdb := newRedis(t)

	ctx := registerAndLogin(t, userConn, uniquePlate("T18"), "CAR")

	reservationID, spotID := createReservationAndWait(t, resConn, ctx, rdb, "SYSTEM_ASSIGNED", "CAR", "")
	t.Logf("✓ Reserved: id=%s spot=%s", reservationID, spotID)

	// Cancel immediately (within 2 min) → fee should be 0
	cancelResp := &reservationpb.CancelReservationResponse{}
	if err := resConn.Invoke(ctx, "/reservation.ReservationService/CancelReservation",
		&reservationpb.CancelReservationRequest{ReservationId: reservationID}, cancelResp); err != nil {
		t.Fatalf("CancelReservation failed: %v", err)
	}

	if cancelResp.Status != "CANCELLED" {
		t.Errorf("expected CANCELLED, got %s", cancelResp.Status)
	}
	if cancelResp.CancellationFee != 0 {
		t.Errorf("expected fee=0, got %d", cancelResp.CancellationFee)
	}
	t.Logf("✓ Cancelled: fee=%d status=%s", cancelResp.CancellationFee, cancelResp.Status)

	// Verify spot released
	_, err := rdb.Get(context.Background(), "lock:"+spotID).Result()
	if err != nil {
		t.Logf("✓ Lock released for spot %s", spotID)
	} else {
		t.Error("✗ Lock should have been released")
	}

	t.Log("✓ PASS: Task 18 — Cancel ≤ 2 min, fee=0")
}

func TestTask19_CancelWithFee(t *testing.T) {
	userConn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	resConn := dialGRPC(t, envOr("RESERVATION_ADDR", "localhost:50052"))
	rdb := newRedis(t)
	db := connectDB(t, "DATABASE_URL", "postgres://parkir:parkir@localhost:5433/reservation_db?sslmode=disable")

	ctx := registerAndLogin(t, userConn, uniquePlate("T19"), "CAR")

	reservationID, spotID := createReservationAndWait(t, resConn, ctx, rdb, "SYSTEM_ASSIGNED", "CAR", "")
	t.Logf("✓ Reserved: id=%s spot=%s", reservationID, spotID)

	// Simulate > 2 min elapsed by backdating confirmed_at
	_, err := db.Exec(context.Background(),
		"UPDATE reservations SET confirmed_at = now() - interval '3 minutes' WHERE id = $1", reservationID)
	if err != nil && err != pgx.ErrNoRows {
		t.Fatalf("update confirmed_at: %v", err)
	}
	t.Log("✓ confirmed_at backdated by 3 minutes")

	time.Sleep(500 * time.Millisecond) // let DB settle

	cancelResp := &reservationpb.CancelReservationResponse{}
	if err := resConn.Invoke(ctx, "/reservation.ReservationService/CancelReservation",
		&reservationpb.CancelReservationRequest{ReservationId: reservationID}, cancelResp); err != nil {
		t.Fatalf("CancelReservation failed: %v", err)
	}

	if cancelResp.Status != "CANCELLED" {
		t.Errorf("expected CANCELLED, got %s", cancelResp.Status)
	}
	if cancelResp.CancellationFee != 5000 {
		t.Errorf("expected fee=5000, got %d", cancelResp.CancellationFee)
	}
	t.Logf("✓ Cancelled: fee=%d status=%s", cancelResp.CancellationFee, cancelResp.Status)

	t.Log("✓ PASS: Task 19 — Cancel > 2 min, fee=5.000 IDR")
}
