// Task 17: Wrong spot blocking
//   Check-in at different spot → BLOCKED, cannot park

//go:build integration

package integration

import (
	"testing"

	reservationpb "github.com/parkir-pintar/reservation/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTask17_WrongSpotBlocked(t *testing.T) {
	resConn := dialGRPC(t, envOr("RESERVATION_ADDR", "localhost:50052"))
	rdb := newRedis(t)

	ctx := testContext(t)

	reservationID, spotID := createReservationAndWait(t, resConn, ctx, rdb, "SYSTEM_ASSIGNED", "CAR", "")
	t.Logf("✓ Reserved: id=%s spot=%s", reservationID, spotID)

	// Check-in at a WRONG spot — should be BLOCKED
	wrongSpot := "5-CAR-30" // deliberately different
	if wrongSpot == spotID {
		wrongSpot = "5-CAR-29"
	}

	checkinResp := &reservationpb.CheckInResponse{}
	err := resConn.Invoke(ctx, "/reservation.ReservationService/CheckIn",
		&reservationpb.CheckInRequest{ReservationId: reservationID, ActualSpotId: wrongSpot}, checkinResp)

	// Two valid outcomes:
	// 1. Error with FAILED_PRECONDITION (BLOCKED)
	// 2. Success response with WrongSpot=true
	if err != nil {
		st, ok := status.FromError(err)
		if !ok {
			t.Fatalf("expected gRPC status error, got: %v", err)
		}
		if st.Code() != codes.FailedPrecondition {
			t.Errorf("expected FAILED_PRECONDITION, got %s", st.Code())
		}
		t.Logf("✓ Wrong spot BLOCKED (error): %s", st.Message())
	} else {
		if !checkinResp.WrongSpot {
			t.Error("expected wrong_spot=true")
		}
		t.Logf("✓ Wrong spot BLOCKED (response): wrong_spot=%v", checkinResp.WrongSpot)
	}

	t.Log("✓ PASS: Task 17 — Wrong spot blocked, driver cannot park")
}
