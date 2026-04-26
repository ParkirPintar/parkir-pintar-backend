// Task 17: Wrong spot penalty
//   Check-in at different spot → penalty 200.000 IDR applied

//go:build integration

package integration

import (
	"testing"

	reservationpb "github.com/parkir-pintar/reservation/pkg/proto"
)

func TestTask17_WrongSpotPenalty(t *testing.T) {
	userConn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	resConn := dialGRPC(t, envOr("RESERVATION_ADDR", "localhost:50052"))
	rdb := newRedis(t)

	ctx := registerAndLogin(t, userConn, uniquePlate("T17"), "CAR")

	reservationID, spotID := createReservationAndWait(t, resConn, ctx, rdb, "SYSTEM_ASSIGNED", "CAR", "")
	t.Logf("✓ Reserved: id=%s spot=%s", reservationID, spotID)

	// Check-in at a WRONG spot
	wrongSpot := "5-CAR-30" // deliberately different
	if wrongSpot == spotID {
		wrongSpot = "5-CAR-29"
	}

	checkinResp := &reservationpb.CheckInResponse{}
	if err := resConn.Invoke(ctx, "/reservation.ReservationService/CheckIn",
		&reservationpb.CheckInRequest{ReservationId: reservationID, ActualSpotId: wrongSpot}, checkinResp); err != nil {
		t.Fatalf("CheckIn failed: %v", err)
	}

	if !checkinResp.WrongSpot {
		t.Error("expected wrong_spot=true")
	}
	if checkinResp.PenaltyApplied != 200000 {
		t.Errorf("expected penalty=200000, got %d", checkinResp.PenaltyApplied)
	}
	t.Logf("✓ Wrong spot detected: penalty=%d IDR", checkinResp.PenaltyApplied)
	t.Log("✓ PASS: Task 17 — Wrong spot penalty 200.000 IDR applied")
}
