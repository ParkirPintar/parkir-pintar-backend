// Task 17: Wrong spot blocking
//   Check-in at different spot → BLOCKED, cannot park

//go:build integration

package integration

import (
	"testing"

	presencepb "github.com/parkir-pintar/presence/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTask17_WrongSpotBlocked(t *testing.T) {
	resConn := dialGRPC(t, envOr("RESERVATION_ADDR", "localhost:50052"))
	presenceConn := dialGRPC(t, envOr("PRESENCE_ADDR", "localhost:50056"))
	rdb := newRedis(t)

	ctx := testContext(t)

	reservationID, spotID := createReservationAndWait(t, resConn, ctx, rdb, "SYSTEM_ASSIGNED", "CAR", "")
	t.Logf("✓ Reserved: id=%s spot=%s", reservationID, spotID)

	// Check-in at a WRONG spot — should be BLOCKED
	wrongSpot := "5-CAR-30" // deliberately different
	if wrongSpot == spotID {
		wrongSpot = "5-CAR-29"
	}

	checkinResp := &presencepb.CheckInResponse{}
	err := presenceConn.Invoke(ctx, "/presence.PresenceService/CheckIn",
		&presencepb.CheckInRequest{ReservationId: reservationID, SpotId: wrongSpot}, checkinResp)

	// Should return FAILED_PRECONDITION (BLOCKED)
	if err == nil {
		t.Fatal("expected error for wrong spot, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.FailedPrecondition {
		t.Errorf("expected FAILED_PRECONDITION, got %s", st.Code())
	}

	t.Logf("✓ Wrong spot BLOCKED: %s", st.Message())
	t.Log("✓ PASS: Task 17 — Wrong spot blocked, driver cannot park")
}
