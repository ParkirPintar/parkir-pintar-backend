package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/parkir-pintar/billing/internal/adapter"
	"github.com/parkir-pintar/billing/internal/model"
)

// --- Mock repository ---

type mockBillingRepo struct {
	records          map[string]*model.BillingRecord // keyed by reservation_id
	idempotencyStore map[string]*model.BillingRecord // keyed by idempotency_key
	updateCalls      int
	createCalls      int
	idempotencyKeys  map[string]string // key -> invoice_id
}

func newMockRepo() *mockBillingRepo {
	return &mockBillingRepo{
		records:          make(map[string]*model.BillingRecord),
		idempotencyStore: make(map[string]*model.BillingRecord),
		idempotencyKeys:  make(map[string]string),
	}
}

func (m *mockBillingRepo) Create(_ context.Context, b *model.BillingRecord) error {
	m.createCalls++
	m.records[b.ReservationID] = b
	return nil
}

func (m *mockBillingRepo) GetByReservationID(_ context.Context, reservationID string) (*model.BillingRecord, error) {
	b, ok := m.records[reservationID]
	if !ok {
		return nil, fmt.Errorf("no rows in result set")
	}
	// Return a copy to avoid mutation issues.
	copy := *b
	return &copy, nil
}

func (m *mockBillingRepo) Update(_ context.Context, b *model.BillingRecord) error {
	m.updateCalls++
	m.records[b.ReservationID] = b
	if b.IdempotencyKey != "" {
		m.idempotencyStore[b.IdempotencyKey] = b
	}
	return nil
}

func (m *mockBillingRepo) GetActivePricingRule(_ context.Context) ([]byte, int, error) {
	return nil, 0, nil
}

func (m *mockBillingRepo) GetByIdempotencyKey(_ context.Context, key string) (*model.BillingRecord, error) {
	if id, ok := m.idempotencyKeys[key]; ok {
		for _, b := range m.records {
			if b.ID == id {
				copy := *b
				return &copy, nil
			}
		}
	}
	return nil, nil
}

func (m *mockBillingRepo) SetIdempotencyKey(_ context.Context, key string, invoiceID string) error {
	m.idempotencyKeys[key] = invoiceID
	return nil
}

// --- Mock payment client ---

type mockPaymentClient struct {
	paymentID string
	qrCode    string
	err       error
	calls     int
}

func (m *mockPaymentClient) CreatePayment(_ context.Context, invoiceID string, amount int64, idempotencyKey string) (string, string, error) {
	m.calls++
	if m.err != nil {
		return "", "", m.err
	}
	return m.paymentID, m.qrCode, nil
}

// --- Mock publisher ---

type mockPublisher struct {
	events []publishedEvent
	err    error
}

type publishedEvent struct {
	eventType string
	payload   []byte
}

func (m *mockPublisher) Publish(_ context.Context, eventType string, payload []byte) error {
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, publishedEvent{eventType: eventType, payload: payload})
	return nil
}

// --- Helper to create a usecase with mocks ---

func newTestUsecase(repo *mockBillingRepo, pc adapter.PaymentClient, pub adapter.EventPublisher) BillingUsecase {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately to prevent hotReload goroutine from running
	return NewBillingUsecase(ctx, repo, pc, pub)
}

// --- Tests ---

func TestCheckout_HappyPath(t *testing.T) {
	repo := newMockRepo()
	pc := &mockPaymentClient{paymentID: "pay-123", qrCode: "qr-abc"}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, pc, pub)

	// Create a billing record with a session start.
	start := time.Now().Add(-2 * time.Hour)
	repo.records["res-1"] = &model.BillingRecord{
		ID:            "inv-1",
		ReservationID: "res-1",
		BookingFee:    5000,
		Status:        model.BillingPending,
		SessionStart:  &start,
	}

	b, err := uc.Checkout(context.Background(), "res-1", "idem-key-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify fees are calculated.
	if b.HourlyFee <= 0 {
		t.Errorf("expected positive hourly fee, got %d", b.HourlyFee)
	}
	if b.BookingFee != 5000 {
		t.Errorf("booking fee = %d, want 5000", b.BookingFee)
	}
	if b.Total <= 0 {
		t.Errorf("expected positive total, got %d", b.Total)
	}

	// Verify payment was called.
	if pc.calls != 1 {
		t.Errorf("payment client calls = %d, want 1", pc.calls)
	}
	if b.PaymentID != "pay-123" {
		t.Errorf("payment_id = %q, want %q", b.PaymentID, "pay-123")
	}
	if b.QRCode != "qr-abc" {
		t.Errorf("qr_code = %q, want %q", b.QRCode, "qr-abc")
	}

	// Verify idempotency key is set on the record.
	if b.IdempotencyKey != "idem-key-1" {
		t.Errorf("idempotency_key = %q, want %q", b.IdempotencyKey, "idem-key-1")
	}

	// Verify checkout.completed event was published.
	if len(pub.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(pub.events))
	}
	if pub.events[0].eventType != "checkout.completed" {
		t.Errorf("event type = %q, want %q", pub.events[0].eventType, "checkout.completed")
	}

	// Verify event payload contains expected fields.
	var payload map[string]any
	if err := json.Unmarshal(pub.events[0].payload, &payload); err != nil {
		t.Fatalf("failed to unmarshal event payload: %v", err)
	}
	if payload["invoice_id"] != "inv-1" {
		t.Errorf("event invoice_id = %v, want %q", payload["invoice_id"], "inv-1")
	}
	if payload["payment_id"] != "pay-123" {
		t.Errorf("event payment_id = %v, want %q", payload["payment_id"], "pay-123")
	}
}

func TestCheckout_IdempotencyReturnsExisting(t *testing.T) {
	repo := newMockRepo()
	pc := &mockPaymentClient{paymentID: "pay-123", qrCode: "qr-abc"}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, pc, pub)

	// Create a billing record with a session start.
	start := time.Now().Add(-1 * time.Hour)
	repo.records["res-1"] = &model.BillingRecord{
		ID:             "inv-1",
		ReservationID:  "res-1",
		BookingFee:     5000,
		Status:         model.BillingPending,
		SessionStart:   &start,
		IdempotencyKey: "idem-key-1",
	}

	// First checkout.
	b1, err := uc.Checkout(context.Background(), "res-1", "idem-key-1")
	if err != nil {
		t.Fatalf("first checkout error: %v", err)
	}

	// Second checkout with same idempotency key should return cached result.
	b2, err := uc.Checkout(context.Background(), "res-1", "idem-key-1")
	if err != nil {
		t.Fatalf("second checkout error: %v", err)
	}

	if b1.ID != b2.ID {
		t.Errorf("idempotent checkout returned different IDs: %q vs %q", b1.ID, b2.ID)
	}

	// Payment should only be called once.
	if pc.calls != 1 {
		t.Errorf("payment client calls = %d, want 1", pc.calls)
	}
}

func TestCheckout_NotFoundReservation(t *testing.T) {
	repo := newMockRepo()
	pc := &mockPaymentClient{paymentID: "pay-123", qrCode: "qr-abc"}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, pc, pub)

	_, err := uc.Checkout(context.Background(), "nonexistent-res", "idem-key-1")
	if err == nil {
		t.Fatal("expected error for non-existent reservation, got nil")
	}
}

func TestCheckout_PaymentFailure_PublishesFailedEvent(t *testing.T) {
	repo := newMockRepo()
	pc := &mockPaymentClient{err: fmt.Errorf("payment service unavailable")}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, pc, pub)

	start := time.Now().Add(-1 * time.Hour)
	repo.records["res-1"] = &model.BillingRecord{
		ID:            "inv-1",
		ReservationID: "res-1",
		BookingFee:    5000,
		Status:        model.BillingPending,
		SessionStart:  &start,
	}

	_, err := uc.Checkout(context.Background(), "res-1", "idem-key-1")
	if err == nil {
		t.Fatal("expected error when payment fails, got nil")
	}

	// Verify checkout.failed event was published.
	if len(pub.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(pub.events))
	}
	if pub.events[0].eventType != "checkout.failed" {
		t.Errorf("event type = %q, want %q", pub.events[0].eventType, "checkout.failed")
	}

	// Verify billing record status is FAILED.
	updated := repo.records["res-1"]
	if updated.Status != model.BillingFailed {
		t.Errorf("status = %q, want %q", updated.Status, model.BillingFailed)
	}
}

func TestCheckout_NilPaymentClient_GracefulDegradation(t *testing.T) {
	repo := newMockRepo()
	pub := &mockPublisher{}

	// Pass nil payment client.
	uc := newTestUsecase(repo, nil, pub)

	start := time.Now().Add(-1 * time.Hour)
	repo.records["res-1"] = &model.BillingRecord{
		ID:            "inv-1",
		ReservationID: "res-1",
		BookingFee:    5000,
		Status:        model.BillingPending,
		SessionStart:  &start,
	}

	b, err := uc.Checkout(context.Background(), "res-1", "idem-key-1")
	if err != nil {
		t.Fatalf("unexpected error with nil payment client: %v", err)
	}

	// Payment fields should be empty.
	if b.PaymentID != "" {
		t.Errorf("payment_id = %q, want empty", b.PaymentID)
	}
	if b.QRCode != "" {
		t.Errorf("qr_code = %q, want empty", b.QRCode)
	}

	// checkout.completed should still be published.
	if len(pub.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(pub.events))
	}
	if pub.events[0].eventType != "checkout.completed" {
		t.Errorf("event type = %q, want %q", pub.events[0].eventType, "checkout.completed")
	}
}

func TestCheckout_NilPublisher_GracefulDegradation(t *testing.T) {
	repo := newMockRepo()
	pc := &mockPaymentClient{paymentID: "pay-123", qrCode: "qr-abc"}

	// Pass nil publisher.
	uc := newTestUsecase(repo, pc, nil)

	start := time.Now().Add(-1 * time.Hour)
	repo.records["res-1"] = &model.BillingRecord{
		ID:            "inv-1",
		ReservationID: "res-1",
		BookingFee:    5000,
		Status:        model.BillingPending,
		SessionStart:  &start,
	}

	b, err := uc.Checkout(context.Background(), "res-1", "idem-key-1")
	if err != nil {
		t.Fatalf("unexpected error with nil publisher: %v", err)
	}

	// Payment should still work.
	if b.PaymentID != "pay-123" {
		t.Errorf("payment_id = %q, want %q", b.PaymentID, "pay-123")
	}
}

func TestCheckout_IncludesNoshowFee(t *testing.T) {
	repo := newMockRepo()
	pc := &mockPaymentClient{paymentID: "pay-123", qrCode: "qr-abc"}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, pc, pub)

	start := time.Now().Add(-1 * time.Hour)
	repo.records["res-1"] = &model.BillingRecord{
		ID:            "inv-1",
		ReservationID: "res-1",
		BookingFee:    5000,
		NoshowFee:     10000, // Pre-existing noshow fee from expiry worker
		Status:        model.BillingPending,
		SessionStart:  &start,
	}

	b, err := uc.Checkout(context.Background(), "res-1", "idem-key-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Noshow fee should be included in total via pricing engine.
	if b.NoshowFee != 10000 {
		t.Errorf("noshow_fee = %d, want 10000", b.NoshowFee)
	}
	if b.Total <= b.BookingFee+b.HourlyFee {
		t.Errorf("total %d should include noshow_fee", b.Total)
	}
}

func TestCheckout_IncludesPenalty(t *testing.T) {
	repo := newMockRepo()
	pc := &mockPaymentClient{paymentID: "pay-123", qrCode: "qr-abc"}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, pc, pub)

	start := time.Now().Add(-1 * time.Hour)
	repo.records["res-1"] = &model.BillingRecord{
		ID:            "inv-1",
		ReservationID: "res-1",
		BookingFee:    5000,
		Penalty:       200000, // Pre-existing wrong-spot penalty
		Status:        model.BillingPending,
		SessionStart:  &start,
	}

	b, err := uc.Checkout(context.Background(), "res-1", "idem-key-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Penalty should be reflected in total via pricing engine (wrong_spot flag).
	if b.Total < 200000 {
		t.Errorf("total %d should include penalty of 200000", b.Total)
	}
}

func TestChargeBookingFee(t *testing.T) {
	repo := newMockRepo()
	uc := newTestUsecase(repo, nil, nil)

	b, err := uc.ChargeBookingFee(context.Background(), "res-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if b.BookingFee != 5000 {
		t.Errorf("booking_fee = %d, want 5000", b.BookingFee)
	}
	if b.Status != model.BillingPending {
		t.Errorf("status = %q, want %q", b.Status, model.BillingPending)
	}
	if b.ID == "" {
		t.Error("expected non-empty ID")
	}
	if repo.createCalls != 1 {
		t.Errorf("create calls = %d, want 1", repo.createCalls)
	}
}

func TestStartSession(t *testing.T) {
	repo := newMockRepo()
	uc := newTestUsecase(repo, nil, nil)

	repo.records["res-1"] = &model.BillingRecord{
		ID:            "inv-1",
		ReservationID: "res-1",
		BookingFee:    5000,
		Status:        model.BillingPending,
	}

	checkinAt := time.Now()
	err := uc.StartSession(context.Background(), "res-1", checkinAt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := repo.records["res-1"]
	if updated.SessionStart == nil {
		t.Fatal("expected session_start to be set")
	}
	if !updated.SessionStart.Equal(checkinAt) {
		t.Errorf("session_start = %v, want %v", updated.SessionStart, checkinAt)
	}
}

func TestApplyPenalty(t *testing.T) {
	repo := newMockRepo()
	uc := newTestUsecase(repo, nil, nil)

	repo.records["res-1"] = &model.BillingRecord{
		ID:            "inv-1",
		ReservationID: "res-1",
		BookingFee:    5000,
		Status:        model.BillingPending,
	}

	err := uc.ApplyPenalty(context.Background(), "res-1", "wrong_spot", 200000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := repo.records["res-1"]
	if updated.Penalty != 200000 {
		t.Errorf("penalty = %d, want 200000", updated.Penalty)
	}
}
