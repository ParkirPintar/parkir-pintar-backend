package usecase

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/parkir-pintar/reservation/internal/model"
)

// ─── Mock implementations ───────────────────────────────────────────

type mockRepo struct {
	reservations    map[string]*model.Reservation
	idempotencyKeys map[string]string
	holdOwners      map[string]string
	lockedSpots     map[string]bool
	lockErr         error
	holdErr         error
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		reservations:    make(map[string]*model.Reservation),
		idempotencyKeys: make(map[string]string),
		holdOwners:      make(map[string]string),
		lockedSpots:     make(map[string]bool),
	}
}

func (m *mockRepo) Create(_ context.Context, r *model.Reservation) error {
	m.reservations[r.ID] = r
	return nil
}

func (m *mockRepo) GetByID(_ context.Context, id string) (*model.Reservation, error) {
	r, ok := m.reservations[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return r, nil
}

func (m *mockRepo) UpdateStatus(_ context.Context, id string, status model.ReservationStatus) error {
	if r, ok := m.reservations[id]; ok {
		r.Status = status
	}
	return nil
}

func (m *mockRepo) GetIdempotency(_ context.Context, key string) (string, error) {
	if v, ok := m.idempotencyKeys[key]; ok {
		return v, nil
	}
	return "", fmt.Errorf("not found")
}

func (m *mockRepo) SetIdempotency(_ context.Context, key, reservationID string) error {
	m.idempotencyKeys[key] = reservationID
	return nil
}

func (m *mockRepo) HoldSpot(_ context.Context, spotID, driverID string) error {
	if m.holdErr != nil {
		return m.holdErr
	}
	if _, held := m.holdOwners[spotID]; held {
		return fmt.Errorf("spot %s already held", spotID)
	}
	m.holdOwners[spotID] = driverID
	return nil
}

func (m *mockRepo) ReleaseHold(_ context.Context, spotID string) error {
	delete(m.holdOwners, spotID)
	return nil
}

func (m *mockRepo) LockSpot(_ context.Context, spotID string) error {
	if m.lockErr != nil {
		return m.lockErr
	}
	if m.lockedSpots[spotID] {
		return fmt.Errorf("spot %s already locked", spotID)
	}
	m.lockedSpots[spotID] = true
	return nil
}

func (m *mockRepo) ReleaseLock(_ context.Context, spotID string) error {
	delete(m.lockedSpots, spotID)
	return nil
}

func (m *mockRepo) GetHoldOwner(_ context.Context, spotID string) (string, error) {
	if owner, ok := m.holdOwners[spotID]; ok {
		return owner, nil
	}
	return "", nil
}

func (m *mockRepo) SetCheckinAt(_ context.Context, id string, t time.Time) error {
	if r, ok := m.reservations[id]; ok {
		r.CheckinAt = &t
		r.Status = model.StatusActive
	}
	return nil
}

func (m *mockRepo) GetExpiredReservations(_ context.Context) ([]*model.Reservation, error) {
	return nil, nil
}

type mockSearchClient struct {
	spotID string
	err    error
}

func (m *mockSearchClient) GetFirstAvailable(_ context.Context, _ string) (string, error) {
	return m.spotID, m.err
}

type mockBillingClient struct {
	chargeErr      error
	penaltyErr     error
	startErr       error
	paymentID      string
	qrCode         string
	penaltyCalled  bool
	penaltyAmount  int64
}

func (m *mockBillingClient) ChargeBookingFee(_ context.Context, _ string) (string, string, error) {
	return m.paymentID, m.qrCode, m.chargeErr
}

func (m *mockBillingClient) ApplyPenalty(_ context.Context, _ string, _ string, amount int64) error {
	m.penaltyCalled = true
	m.penaltyAmount = amount
	return m.penaltyErr
}

func (m *mockBillingClient) StartBillingSession(_ context.Context, _ string, _ time.Time) error {
	return m.startErr
}

type mockPublisher struct {
	bookingMessages [][]byte
	events          []string
	publishErr      error
}

func (m *mockPublisher) PublishBooking(_ context.Context, _ string, payload []byte) error {
	if m.publishErr != nil {
		return m.publishErr
	}
	m.bookingMessages = append(m.bookingMessages, payload)
	return nil
}

func (m *mockPublisher) PublishEvent(_ context.Context, eventType string, _ []byte) error {
	m.events = append(m.events, eventType)
	return nil
}

// ─── Overlap / Double-Booking Detection Tests ───────────────────────

// TestOverlapDetection_SameSpotLocked verifies that when a spot is already
// locked (reserved by another driver), a second reservation attempt for the
// same spot is rejected. This is the core overlap detection mechanism.
func TestOverlapDetection_SameSpotLocked(t *testing.T) {
	repo := newMockRepo()
	search := &mockSearchClient{spotID: "1-CAR-01"}
	billing := &mockBillingClient{paymentID: "pay-1", qrCode: "qr-1"}
	pub := &mockPublisher{}

	uc := NewReservationUsecase(repo, search, billing, pub)

	// Simulate spot already locked by another driver (overlap scenario)
	repo.lockedSpots["1-CAR-01"] = true

	// Driver B tries to hold the same spot — should fail because it's locked
	// The hold check uses GetHoldOwner which checks the hold key, but the
	// actual overlap prevention is at the lock level in the queue worker.
	// For user-selected mode, the hold mechanism prevents overlap at selection time.
	repo.holdOwners["1-CAR-01"] = "driver-A"

	// Driver B tries user-selected reservation on same spot
	_, err := uc.CreateReservation(context.Background(), "driver-B", "USER_SELECTED", "CAR", "1-CAR-01", "idem-B")
	if err == nil {
		t.Fatal("expected error when spot is held by another driver, got nil")
	}
	t.Logf("✓ Overlap detected: %v", err)
}

// TestOverlapDetection_HoldPreventsDoubleSelection verifies that when Driver A
// holds a spot, Driver B cannot hold the same spot (FIFO hold queue).
func TestOverlapDetection_HoldPreventsDoubleSelection(t *testing.T) {
	repo := newMockRepo()
	search := &mockSearchClient{spotID: "2-CAR-05"}
	billing := &mockBillingClient{}
	pub := &mockPublisher{}

	uc := NewReservationUsecase(repo, search, billing, pub)

	// Driver A holds spot successfully
	_, err := uc.HoldSpot(context.Background(), "2-CAR-05", "driver-A")
	if err != nil {
		t.Fatalf("Driver A hold should succeed: %v", err)
	}

	// Driver B tries to hold the same spot — should fail (overlap)
	_, err = uc.HoldSpot(context.Background(), "2-CAR-05", "driver-B")
	if err == nil {
		t.Fatal("expected error when spot already held by Driver A, got nil")
	}
	t.Logf("✓ Hold overlap detected: %v", err)
}

// TestOverlapDetection_IdempotencyPreventsDoubleBooking verifies that the same
// idempotency key returns the existing reservation without creating a duplicate.
func TestOverlapDetection_IdempotencyPreventsDoubleBooking(t *testing.T) {
	repo := newMockRepo()
	search := &mockSearchClient{spotID: "1-CAR-01"}
	billing := &mockBillingClient{paymentID: "pay-1", qrCode: "qr-1"}
	pub := &mockPublisher{}

	uc := NewReservationUsecase(repo, search, billing, pub)

	// Simulate a previously processed reservation with this idempotency key
	existingRes := &model.Reservation{
		ID:             "res-existing",
		DriverID:       "driver-A",
		SpotID:         "1-CAR-01",
		Mode:           model.ModeSystemAssigned,
		Status:         model.StatusReserved,
		BookingFee:     5000,
		IdempotencyKey: "idem-duplicate",
	}
	repo.reservations["res-existing"] = existingRes
	repo.idempotencyKeys["idem-duplicate"] = "res-existing"

	// Same driver sends duplicate request with same idempotency key
	res, err := uc.CreateReservation(context.Background(), "driver-A", "SYSTEM_ASSIGNED", "CAR", "", "idem-duplicate")
	if err != nil {
		t.Fatalf("idempotent request should not error: %v", err)
	}
	if res.ID != "res-existing" {
		t.Errorf("expected existing reservation ID 'res-existing', got '%s'", res.ID)
	}
	t.Logf("✓ Idempotency prevents double-booking: returned existing res=%s", res.ID)
}

// TestOverlapDetection_SystemAssignedSkipsHeldSpot verifies that system-assigned
// mode detects when the selected spot is already held and rejects it.
func TestOverlapDetection_SystemAssignedSkipsHeldSpot(t *testing.T) {
	repo := newMockRepo()
	search := &mockSearchClient{spotID: "3-CAR-10"}
	billing := &mockBillingClient{}
	pub := &mockPublisher{}

	uc := NewReservationUsecase(repo, search, billing, pub)

	// Spot is already held by another driver
	repo.holdOwners["3-CAR-10"] = "driver-X"

	// System-assigned mode picks this spot but detects it's held
	_, err := uc.CreateReservation(context.Background(), "driver-Y", "SYSTEM_ASSIGNED", "CAR", "", "idem-Y")
	if err == nil {
		t.Fatal("expected error when system-assigned spot is already held, got nil")
	}
	t.Logf("✓ System-assigned overlap detected: %v", err)
}

// TestOverlapDetection_UserSelectedExpiredHold verifies that when a hold expires,
// the reservation attempt is rejected (no stale hold exploitation).
func TestOverlapDetection_UserSelectedExpiredHold(t *testing.T) {
	repo := newMockRepo()
	search := &mockSearchClient{spotID: "1-MOTO-01"}
	billing := &mockBillingClient{}
	pub := &mockPublisher{}

	uc := NewReservationUsecase(repo, search, billing, pub)

	// No hold exists (expired) — user-selected should fail
	_, err := uc.CreateReservation(context.Background(), "driver-A", "USER_SELECTED", "CAR", "1-MOTO-01", "idem-expired")
	if err == nil {
		t.Fatal("expected HOLD_EXPIRED error when no active hold exists, got nil")
	}
	t.Logf("✓ Expired hold detected: %v", err)
}

// TestOverlapDetection_ConcurrentBookingSameSpot simulates two drivers trying
// to book the same spot. The first succeeds (publishes to queue), the second
// is rejected because the hold belongs to a different driver.
func TestOverlapDetection_ConcurrentBookingSameSpot(t *testing.T) {
	repo := newMockRepo()
	search := &mockSearchClient{spotID: "4-CAR-01"}
	billing := &mockBillingClient{}
	pub := &mockPublisher{}

	uc := NewReservationUsecase(repo, search, billing, pub)

	// Driver A holds and reserves
	repo.holdOwners["4-CAR-01"] = "driver-A"
	resA, err := uc.CreateReservation(context.Background(), "driver-A", "USER_SELECTED", "CAR", "4-CAR-01", "idem-A")
	if err != nil {
		t.Fatalf("Driver A reservation should succeed: %v", err)
	}
	t.Logf("✓ Driver A reserved: spot=%s", resA.SpotID)

	// Driver B tries same spot with their own hold — but hold belongs to A
	_, err = uc.CreateReservation(context.Background(), "driver-B", "USER_SELECTED", "CAR", "4-CAR-01", "idem-B")
	if err == nil {
		t.Fatal("expected error for Driver B (hold belongs to Driver A), got nil")
	}
	t.Logf("✓ Concurrent overlap prevented: %v", err)
}

// ─── Cancellation Tests ─────────────────────────────────────────────

func TestCancelReservation_Within2Min_FreeCancellation(t *testing.T) {
	repo := newMockRepo()
	billing := &mockBillingClient{}
	pub := &mockPublisher{}

	uc := NewReservationUsecase(repo, nil, billing, pub)

	// Create a reservation confirmed just now
	res := &model.Reservation{
		ID:          "res-cancel-free",
		DriverID:    "driver-1",
		SpotID:      "1-CAR-01",
		Status:      model.StatusReserved,
		ConfirmedAt: time.Now(), // just confirmed
	}
	repo.reservations["res-cancel-free"] = res
	repo.lockedSpots["1-CAR-01"] = true

	fee, err := uc.CancelReservation(context.Background(), "res-cancel-free")
	if err != nil {
		t.Fatalf("cancel should succeed: %v", err)
	}
	if fee != 0 {
		t.Errorf("expected fee=0 for cancel within 2 min, got %d", fee)
	}
	if repo.lockedSpots["1-CAR-01"] {
		t.Error("expected lock to be released after cancellation")
	}
	t.Logf("✓ Free cancellation: fee=%d", fee)
}

func TestCancelReservation_After2Min_ChargesFee(t *testing.T) {
	repo := newMockRepo()
	billing := &mockBillingClient{}
	pub := &mockPublisher{}

	uc := NewReservationUsecase(repo, nil, billing, pub)

	// Create a reservation confirmed 5 minutes ago
	res := &model.Reservation{
		ID:          "res-cancel-fee",
		DriverID:    "driver-1",
		SpotID:      "2-CAR-01",
		Status:      model.StatusReserved,
		ConfirmedAt: time.Now().Add(-5 * time.Minute),
	}
	repo.reservations["res-cancel-fee"] = res
	repo.lockedSpots["2-CAR-01"] = true

	fee, err := uc.CancelReservation(context.Background(), "res-cancel-fee")
	if err != nil {
		t.Fatalf("cancel should succeed: %v", err)
	}
	if fee != 5000 {
		t.Errorf("expected fee=5000 for cancel after 2 min, got %d", fee)
	}
	if billing.penaltyCalled && billing.penaltyAmount != 5000 {
		t.Errorf("expected billing penalty=5000, got %d", billing.penaltyAmount)
	}
	t.Logf("✓ Cancellation fee charged: fee=%d", fee)
}

// ─── CheckIn Tests ──────────────────────────────────────────────────

func TestCheckIn_CorrectSpot_Success(t *testing.T) {
	repo := newMockRepo()
	billing := &mockBillingClient{}
	pub := &mockPublisher{}

	uc := NewReservationUsecase(repo, nil, billing, pub)

	res := &model.Reservation{
		ID:       "res-checkin",
		DriverID: "driver-1",
		SpotID:   "1-CAR-01",
		Status:   model.StatusReserved,
	}
	repo.reservations["res-checkin"] = res
	repo.lockedSpots["1-CAR-01"] = true

	result, wrongSpot, _, err := uc.CheckIn(context.Background(), "res-checkin", "1-CAR-01")
	if err != nil {
		t.Fatalf("check-in should succeed: %v", err)
	}
	if wrongSpot {
		t.Error("expected wrong_spot=false for correct spot")
	}
	if result.Status != model.StatusActive {
		t.Errorf("expected status ACTIVE, got %s", result.Status)
	}
	t.Logf("✓ Check-in success: status=%s", result.Status)
}

func TestCheckIn_WrongSpot_Blocked(t *testing.T) {
	repo := newMockRepo()
	billing := &mockBillingClient{}
	pub := &mockPublisher{}

	uc := NewReservationUsecase(repo, nil, billing, pub)

	res := &model.Reservation{
		ID:       "res-wrong",
		DriverID: "driver-1",
		SpotID:   "1-CAR-01",
		Status:   model.StatusReserved,
	}
	repo.reservations["res-wrong"] = res

	_, wrongSpot, _, err := uc.CheckIn(context.Background(), "res-wrong", "2-CAR-05")
	if err == nil {
		t.Fatal("expected error for wrong spot, got nil")
	}
	if !wrongSpot {
		t.Error("expected wrong_spot=true")
	}
	t.Logf("✓ Wrong spot blocked: %v", err)
}

func TestCheckIn_NotReservedStatus_Rejected(t *testing.T) {
	repo := newMockRepo()
	billing := &mockBillingClient{}
	pub := &mockPublisher{}

	uc := NewReservationUsecase(repo, nil, billing, pub)

	res := &model.Reservation{
		ID:       "res-active",
		DriverID: "driver-1",
		SpotID:   "1-CAR-01",
		Status:   model.StatusActive, // already checked in
	}
	repo.reservations["res-active"] = res

	_, _, _, err := uc.CheckIn(context.Background(), "res-active", "1-CAR-01")
	if err == nil {
		t.Fatal("expected error for non-RESERVED status, got nil")
	}
	t.Logf("✓ Non-RESERVED status rejected: %v", err)
}

// ─── Invalid Mode Test ──────────────────────────────────────────────

func TestCreateReservation_InvalidMode_Error(t *testing.T) {
	repo := newMockRepo()
	search := &mockSearchClient{spotID: "1-CAR-01"}
	billing := &mockBillingClient{}
	pub := &mockPublisher{}

	uc := NewReservationUsecase(repo, search, billing, pub)

	_, err := uc.CreateReservation(context.Background(), "driver-1", "INVALID_MODE", "CAR", "", "idem-invalid")
	if err == nil {
		t.Fatal("expected error for invalid mode, got nil")
	}
	t.Logf("✓ Invalid mode rejected: %v", err)
}
