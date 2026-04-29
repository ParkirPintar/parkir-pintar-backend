package usecase

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/parkir-pintar/presence/internal/model"
)

// mockReservationClient is a test double for the ReservationClient interface.
type mockReservationClient struct {
	reservedSpotID string
	checkInCalled  bool
	checkInErr     error
	getResErr      error
}

func (m *mockReservationClient) CheckIn(_ context.Context, _, _ string) error {
	m.checkInCalled = true
	return m.checkInErr
}

func (m *mockReservationClient) GetReservation(_ context.Context, _ string) (string, error) {
	if m.getResErr != nil {
		return "", m.getResErr
	}
	return m.reservedSpotID, nil
}

// mockBillingClient is a test double for the BillingClient interface.
type mockBillingClient struct {
	startSessionCalled bool
	startSessionErr    error
}

func (m *mockBillingClient) StartBillingSession(_ context.Context, _ string, _ time.Time) error {
	m.startSessionCalled = true
	return m.startSessionErr
}

func TestProcessLocation_ReturnsLocationUpdated(t *testing.T) {
	resMock := &mockReservationClient{reservedSpotID: "1-CAR-01"}
	bilMock := &mockBillingClient{}
	uc := NewPresenceUsecase(resMock, bilMock)

	event, err := uc.ProcessLocation(context.Background(), model.LocationUpdate{
		ReservationID: "res-1",
		Latitude:      -6.2,
		Longitude:     106.8,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.Event != "LOCATION_UPDATED" {
		t.Errorf("expected LOCATION_UPDATED, got %s", event.Event)
	}
	if event.ReservationID != "res-1" {
		t.Errorf("expected reservation_id res-1, got %s", event.ReservationID)
	}
}

func TestCheckIn_CorrectSpot_Success(t *testing.T) {
	resMock := &mockReservationClient{reservedSpotID: "1-CAR-01"}
	bilMock := &mockBillingClient{}
	uc := NewPresenceUsecase(resMock, bilMock)

	result, err := uc.CheckIn(context.Background(), "res-1", "1-CAR-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "ACTIVE" {
		t.Errorf("expected status ACTIVE, got %s", result.Status)
	}
	if result.WrongSpot {
		t.Error("expected wrong_spot=false")
	}
	if result.CheckinAt == "" {
		t.Error("expected non-empty checkin_at")
	}
	if !resMock.checkInCalled {
		t.Error("expected Reservation.CheckIn to be called")
	}
	if !bilMock.startSessionCalled {
		t.Error("expected Billing.StartBillingSession to be called")
	}
}

func TestCheckIn_WrongSpot_Blocked(t *testing.T) {
	resMock := &mockReservationClient{reservedSpotID: "1-CAR-01"}
	bilMock := &mockBillingClient{}
	uc := NewPresenceUsecase(resMock, bilMock)

	result, err := uc.CheckIn(context.Background(), "res-1", "2-CAR-05")
	if err == nil {
		t.Fatal("expected error for wrong spot, got nil")
	}
	if result == nil {
		t.Fatal("expected result even on wrong spot")
	}
	if result.Status != "BLOCKED" {
		t.Errorf("expected status BLOCKED, got %s", result.Status)
	}
	if !result.WrongSpot {
		t.Error("expected wrong_spot=true")
	}
	if resMock.checkInCalled {
		t.Error("Reservation.CheckIn should NOT be called for wrong spot")
	}
	if bilMock.startSessionCalled {
		t.Error("Billing.StartBillingSession should NOT be called for wrong spot")
	}
}

func TestCheckIn_ReservationNotFound_Error(t *testing.T) {
	resMock := &mockReservationClient{getResErr: fmt.Errorf("not found")}
	bilMock := &mockBillingClient{}
	uc := NewPresenceUsecase(resMock, bilMock)

	_, err := uc.CheckIn(context.Background(), "res-nonexistent", "1-CAR-01")
	if err == nil {
		t.Fatal("expected error for non-existent reservation, got nil")
	}
}

func TestCheckIn_BillingFailure_NonFatal(t *testing.T) {
	resMock := &mockReservationClient{reservedSpotID: "1-CAR-01"}
	bilMock := &mockBillingClient{startSessionErr: fmt.Errorf("billing unavailable")}
	uc := NewPresenceUsecase(resMock, bilMock)

	result, err := uc.CheckIn(context.Background(), "res-1", "1-CAR-01")
	if err != nil {
		t.Fatalf("billing failure should be non-fatal, got error: %v", err)
	}
	if result.Status != "ACTIVE" {
		t.Errorf("expected status ACTIVE even with billing failure, got %s", result.Status)
	}
	if !resMock.checkInCalled {
		t.Error("expected Reservation.CheckIn to be called")
	}
	if !bilMock.startSessionCalled {
		t.Error("expected Billing.StartBillingSession to be called (even if it fails)")
	}
}
