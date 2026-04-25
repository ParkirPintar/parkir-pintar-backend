package usecase

import (
	"context"
	"testing"

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

func TestLoadGeofences(t *testing.T) {
	geofences, err := LoadGeofences("../../configs/geofences.json")
	if err != nil {
		t.Fatalf("LoadGeofences: %v", err)
	}
	if len(geofences) != 400 {
		t.Fatalf("expected 400 geofences, got %d", len(geofences))
	}

	// Verify a known spot exists.
	gf, ok := geofences["1-CAR-01"]
	if !ok {
		t.Fatal("expected spot 1-CAR-01 to exist")
	}
	if gf.RadiusM != 5.0 {
		t.Errorf("expected radius 5.0, got %f", gf.RadiusM)
	}

	// Verify all floors and types are present.
	carCount, motoCount := 0, 0
	for _, g := range geofences {
		if g.SpotID == "" {
			t.Error("empty spot_id found")
		}
		if g.RadiusM != 5.0 {
			t.Errorf("spot %s: expected radius 5.0, got %f", g.SpotID, g.RadiusM)
		}
		if len(g.SpotID) > 4 && g.SpotID[2:5] == "CAR" {
			carCount++
		} else {
			motoCount++
		}
	}
	if carCount != 150 {
		t.Errorf("expected 150 CAR spots, got %d", carCount)
	}
	if motoCount != 250 {
		t.Errorf("expected 250 MOTO spots, got %d", motoCount)
	}
}

func TestProcessLocation_EnterReservedSpot_CheckinTriggered(t *testing.T) {
	mock := &mockReservationClient{reservedSpotID: "1-CAR-01"}
	geofences := map[string]model.SpotGeofence{
		"1-CAR-01": {SpotID: "1-CAR-01", Latitude: -6.2, Longitude: 106.8, RadiusM: 5.0},
	}
	uc := NewPresenceUsecase(mock, geofences)

	event, err := uc.ProcessLocation(context.Background(), "stream-1", model.LocationUpdate{
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
	if event.Event != "CHECKIN_TRIGGERED" {
		t.Errorf("expected CHECKIN_TRIGGERED, got %s", event.Event)
	}
	if event.SpotID != "1-CAR-01" {
		t.Errorf("expected spot 1-CAR-01, got %s", event.SpotID)
	}
	if !mock.checkInCalled {
		t.Error("expected CheckIn to be called")
	}
}

func TestProcessLocation_EnterWrongSpot_WrongSpotDetected(t *testing.T) {
	mock := &mockReservationClient{reservedSpotID: "1-CAR-02"}
	geofences := map[string]model.SpotGeofence{
		"1-CAR-01": {SpotID: "1-CAR-01", Latitude: -6.2, Longitude: 106.8, RadiusM: 5.0},
	}
	uc := NewPresenceUsecase(mock, geofences)

	event, err := uc.ProcessLocation(context.Background(), "stream-1", model.LocationUpdate{
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
	if event.Event != "WRONG_SPOT_DETECTED" {
		t.Errorf("expected WRONG_SPOT_DETECTED, got %s", event.Event)
	}
}

func TestProcessLocation_NoGeofenceMatch_NilEvent(t *testing.T) {
	mock := &mockReservationClient{reservedSpotID: "1-CAR-01"}
	geofences := map[string]model.SpotGeofence{
		"1-CAR-01": {SpotID: "1-CAR-01", Latitude: -6.2, Longitude: 106.8, RadiusM: 5.0},
	}
	uc := NewPresenceUsecase(mock, geofences)

	// Location far from any geofence.
	event, err := uc.ProcessLocation(context.Background(), "stream-1", model.LocationUpdate{
		ReservationID: "res-1",
		Latitude:      -6.3,
		Longitude:     106.9,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event != nil {
		t.Errorf("expected nil event, got %+v", event)
	}
}

func TestProcessLocation_ExitGeofence_GeofenceExited(t *testing.T) {
	mock := &mockReservationClient{reservedSpotID: "1-CAR-01"}
	geofences := map[string]model.SpotGeofence{
		"1-CAR-01": {SpotID: "1-CAR-01", Latitude: -6.2, Longitude: 106.8, RadiusM: 5.0},
	}
	uc := NewPresenceUsecase(mock, geofences)

	// First: enter the geofence.
	_, err := uc.ProcessLocation(context.Background(), "stream-1", model.LocationUpdate{
		ReservationID: "res-1",
		Latitude:      -6.2,
		Longitude:     106.8,
	})
	if err != nil {
		t.Fatalf("unexpected error on enter: %v", err)
	}

	// Second: move far away (exit geofence).
	event, err := uc.ProcessLocation(context.Background(), "stream-1", model.LocationUpdate{
		ReservationID: "res-1",
		Latitude:      -6.3,
		Longitude:     106.9,
	})
	if err != nil {
		t.Fatalf("unexpected error on exit: %v", err)
	}
	if event == nil {
		t.Fatal("expected GEOFENCE_EXITED event, got nil")
	}
	if event.Event != "GEOFENCE_EXITED" {
		t.Errorf("expected GEOFENCE_EXITED, got %s", event.Event)
	}
	if event.SpotID != "1-CAR-01" {
		t.Errorf("expected spot 1-CAR-01, got %s", event.SpotID)
	}
}

func TestProcessLocation_StayInSameSpot_NoEvent(t *testing.T) {
	mock := &mockReservationClient{reservedSpotID: "1-CAR-01"}
	geofences := map[string]model.SpotGeofence{
		"1-CAR-01": {SpotID: "1-CAR-01", Latitude: -6.2, Longitude: 106.8, RadiusM: 5.0},
	}
	uc := NewPresenceUsecase(mock, geofences)

	// First: enter the geofence.
	_, err := uc.ProcessLocation(context.Background(), "stream-1", model.LocationUpdate{
		ReservationID: "res-1",
		Latitude:      -6.2,
		Longitude:     106.8,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second: still in the same geofence (slightly different coords but within radius).
	event, err := uc.ProcessLocation(context.Background(), "stream-1", model.LocationUpdate{
		ReservationID: "res-1",
		Latitude:      -6.2000001,
		Longitude:     106.8000001,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event != nil {
		t.Errorf("expected nil event for same spot, got %+v", event)
	}
}

func TestRemoveStream(t *testing.T) {
	mock := &mockReservationClient{reservedSpotID: "1-CAR-01"}
	geofences := map[string]model.SpotGeofence{
		"1-CAR-01": {SpotID: "1-CAR-01", Latitude: -6.2, Longitude: 106.8, RadiusM: 5.0},
	}
	uc := NewPresenceUsecase(mock, geofences)

	// Enter a geofence.
	_, _ = uc.ProcessLocation(context.Background(), "stream-1", model.LocationUpdate{
		ReservationID: "res-1",
		Latitude:      -6.2,
		Longitude:     106.8,
	})

	// Remove stream state.
	uc.RemoveStream("stream-1")

	// Re-entering should trigger a new CHECKIN_TRIGGERED (not "same spot, no event").
	event, err := uc.ProcessLocation(context.Background(), "stream-1", model.LocationUpdate{
		ReservationID: "res-1",
		Latitude:      -6.2,
		Longitude:     106.8,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event == nil {
		t.Fatal("expected event after stream removal, got nil")
	}
	if event.Event != "CHECKIN_TRIGGERED" {
		t.Errorf("expected CHECKIN_TRIGGERED, got %s", event.Event)
	}
}

func TestHaversineM(t *testing.T) {
	// Same point should be 0.
	d := haversineM(-6.2, 106.8, -6.2, 106.8)
	if d != 0 {
		t.Errorf("expected 0 distance for same point, got %f", d)
	}

	// Known distance: ~111km per degree of latitude.
	d = haversineM(0, 0, 1, 0)
	if d < 110000 || d > 112000 {
		t.Errorf("expected ~111km for 1 degree latitude, got %f meters", d)
	}
}
