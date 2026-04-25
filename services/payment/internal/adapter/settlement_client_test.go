package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/parkir-pintar/payment/internal/model"
)

func TestRequestQRIS_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/qris/create" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content type")
		}

		var body qrisRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body.InvoiceID != "inv-123" {
			t.Errorf("expected invoice_id inv-123, got %s", body.InvoiceID)
		}
		if body.Amount != 50000 {
			t.Errorf("expected amount 50000, got %d", body.Amount)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(qrisResponse{QRCode: "QR-ABC-123"})
	}))
	defer srv.Close()

	client := NewSettlementClient(srv.URL)
	qr, err := client.RequestQRIS(context.Background(), "inv-123", 50000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if qr != "QR-ABC-123" {
		t.Errorf("expected QR-ABC-123, got %s", qr)
	}
}

func TestRequestQRIS_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client := NewSettlementClient(srv.URL)
	_, err := client.RequestQRIS(context.Background(), "inv-123", 50000)
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

func TestCheckStatus_Paid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/settlement/pay-456" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(statusResponse{Status: "PAID"})
	}))
	defer srv.Close()

	client := NewSettlementClient(srv.URL)
	status, err := client.CheckStatus(context.Background(), "pay-456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != model.PaymentPaid {
		t.Errorf("expected PAID, got %s", status)
	}
}

func TestCheckStatus_Failed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(statusResponse{Status: "FAILED"})
	}))
	defer srv.Close()

	client := NewSettlementClient(srv.URL)
	status, err := client.CheckStatus(context.Background(), "pay-789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != model.PaymentFailed {
		t.Errorf("expected FAILED, got %s", status)
	}
}

func TestCheckStatus_UnknownDefaultsPending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(statusResponse{Status: "PROCESSING"})
	}))
	defer srv.Close()

	client := NewSettlementClient(srv.URL)
	status, err := client.CheckStatus(context.Background(), "pay-000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != model.PaymentPending {
		t.Errorf("expected PENDING for unknown status, got %s", status)
	}
}

func TestCheckStatus_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	client := NewSettlementClient(srv.URL)
	_, err := client.CheckStatus(context.Background(), "pay-missing")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}
