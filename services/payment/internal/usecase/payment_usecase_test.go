package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/parkir-pintar/payment/internal/model"
)

// --- Mock repository ---

type mockPaymentRepo struct {
	payments       map[string]*model.Payment
	byIdempotency  map[string]*model.Payment
	createErr      error
	getByIDErr     error
	getByIdemErr   error
	updateStatusFn func(id string, status model.PaymentStatus) error
}

func newMockRepo() *mockPaymentRepo {
	return &mockPaymentRepo{
		payments:      make(map[string]*model.Payment),
		byIdempotency: make(map[string]*model.Payment),
	}
}

func (m *mockPaymentRepo) Create(_ context.Context, p *model.Payment) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.payments[p.ID] = p
	if p.IdempotencyKey != "" {
		m.byIdempotency[p.IdempotencyKey] = p
	}
	return nil
}

func (m *mockPaymentRepo) GetByID(_ context.Context, id string) (*model.Payment, error) {
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	p, ok := m.payments[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return p, nil
}

func (m *mockPaymentRepo) GetByIdempotencyKey(_ context.Context, key string) (*model.Payment, error) {
	if m.getByIdemErr != nil {
		return nil, m.getByIdemErr
	}
	p, ok := m.byIdempotency[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return p, nil
}

func (m *mockPaymentRepo) UpdateStatus(_ context.Context, id string, status model.PaymentStatus) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(id, status)
	}
	p, ok := m.payments[id]
	if !ok {
		return errors.New("not found")
	}
	p.Status = status
	return nil
}

// --- Mock settlement client ---

type mockSettlement struct {
	qrCode    string
	qrErr     error
	status    model.PaymentStatus
	statusErr error
	callCount int
}

func (m *mockSettlement) RequestQRIS(_ context.Context, _ string, _ int64) (string, error) {
	m.callCount++
	return m.qrCode, m.qrErr
}

func (m *mockSettlement) CheckStatus(_ context.Context, _ string) (model.PaymentStatus, error) {
	return m.status, m.statusErr
}

// --- Tests ---

func TestCreatePayment_IdempotencyReturnsExisting(t *testing.T) {
	repo := newMockRepo()
	settlement := &mockSettlement{qrCode: "QR-NEW"}

	existing := &model.Payment{
		ID:             "existing-id",
		InvoiceID:      "inv-1",
		Amount:         10000,
		Status:         model.PaymentPending,
		Method:         "QRIS",
		QRCode:         "QR-OLD",
		IdempotencyKey: "idem-key-1",
	}
	repo.payments[existing.ID] = existing
	repo.byIdempotency[existing.IdempotencyKey] = existing

	uc := NewPaymentUsecase(repo, settlement)
	p, err := uc.CreatePayment(context.Background(), "inv-1", 10000, "idem-key-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID != "existing-id" {
		t.Errorf("expected existing payment ID, got %s", p.ID)
	}
	if p.QRCode != "QR-OLD" {
		t.Errorf("expected existing QR code, got %s", p.QRCode)
	}
	if settlement.callCount != 0 {
		t.Errorf("settlement should not be called for idempotent request, called %d times", settlement.callCount)
	}
}

func TestCreatePayment_CircuitBreakerOpenFallback(t *testing.T) {
	repo := newMockRepo()
	settlement := &mockSettlement{qrErr: errors.New("connection refused")}

	uc := NewPaymentUsecase(repo, settlement)

	// Trip the circuit breaker by causing 5 consecutive failures.
	for i := 0; i < 5; i++ {
		_, _ = uc.CreatePayment(context.Background(), "inv-trip", 5000, "")
	}

	// Now the circuit breaker should be OPEN. Next call should return fallback.
	settlement.callCount = 0
	p, err := uc.CreatePayment(context.Background(), "inv-fallback", 5000, "")
	if err != nil {
		t.Fatalf("expected fallback payment, got error: %v", err)
	}
	if p.Status != model.PaymentPending {
		t.Errorf("expected PENDING status, got %s", p.Status)
	}
	if p.QRCode != "" {
		t.Errorf("expected empty QR code in fallback, got %s", p.QRCode)
	}
	if settlement.callCount != 0 {
		t.Errorf("settlement should not be called when circuit breaker is OPEN, called %d times", settlement.callCount)
	}
	// Verify the fallback payment was persisted.
	if _, ok := repo.payments[p.ID]; !ok {
		t.Error("fallback payment should be persisted in repository")
	}
}

func TestCreatePayment_ReadyToTripAt5Failures(t *testing.T) {
	repo := newMockRepo()
	settlement := &mockSettlement{qrErr: errors.New("connection refused")}

	uc := NewPaymentUsecase(repo, settlement)

	// After exactly 4 failures, circuit breaker should still be CLOSED (calls settlement).
	for i := 0; i < 4; i++ {
		_, _ = uc.CreatePayment(context.Background(), "inv-trip", 5000, "")
	}

	// 5th call should still go through the circuit breaker (it trips AFTER this call).
	settlement.callCount = 0
	_, err := uc.CreatePayment(context.Background(), "inv-5th", 5000, "")
	if err == nil {
		t.Fatal("expected error on 5th failure")
	}
	if settlement.callCount != 1 {
		t.Errorf("5th call should still reach settlement (trips after), got callCount=%d", settlement.callCount)
	}

	// 6th call: circuit breaker is now OPEN, should get fallback.
	settlement.callCount = 0
	p, err := uc.CreatePayment(context.Background(), "inv-6th", 5000, "")
	if err != nil {
		t.Fatalf("expected fallback on 6th call, got error: %v", err)
	}
	if p.QRCode != "" {
		t.Errorf("expected empty QR code in fallback, got %s", p.QRCode)
	}
	if settlement.callCount != 0 {
		t.Errorf("settlement should not be called when OPEN, called %d times", settlement.callCount)
	}
}

func TestCreatePayment_HappyPath(t *testing.T) {
	repo := newMockRepo()
	settlement := &mockSettlement{qrCode: "QR-HAPPY"}

	uc := NewPaymentUsecase(repo, settlement)
	p, err := uc.CreatePayment(context.Background(), "inv-happy", 25000, "idem-happy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.QRCode != "QR-HAPPY" {
		t.Errorf("expected QR-HAPPY, got %s", p.QRCode)
	}
	if p.Status != model.PaymentPending {
		t.Errorf("expected PENDING, got %s", p.Status)
	}
	if p.InvoiceID != "inv-happy" {
		t.Errorf("expected inv-happy, got %s", p.InvoiceID)
	}
	if p.Amount != 25000 {
		t.Errorf("expected 25000, got %d", p.Amount)
	}
	if p.IdempotencyKey != "idem-happy" {
		t.Errorf("expected idem-happy, got %s", p.IdempotencyKey)
	}
}
