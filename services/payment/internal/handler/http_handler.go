package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/parkir-pintar/payment/internal/usecase"
	"github.com/rs/zerolog/log"
)

// HTTPHandler exposes REST endpoints for the Payment service.
type HTTPHandler struct {
	uc usecase.PaymentUsecase
}

// NewHTTPHandler creates a new HTTP handler with the given usecase.
func NewHTTPHandler(uc usecase.PaymentUsecase) *HTTPHandler {
	return &HTTPHandler{uc: uc}
}

// Register mounts all REST routes on the given mux.
func (h *HTTPHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/payments/{id}", h.getPaymentStatus)
	mux.HandleFunc("POST /v1/payments/{id}/retry", h.retryPayment)
}

func (h *HTTPHandler) getPaymentStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "payment_id required")
		return
	}

	// Strip /retry suffix if accidentally matched (shouldn't happen with Go 1.22+ routing)
	id = strings.TrimSuffix(id, "/retry")

	p, err := h.uc.GetPaymentStatus(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "payment not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"payment_id": p.ID,
		"invoice_id": p.InvoiceID,
		"status":     string(p.Status),
		"amount":     p.Amount,
		"method":     p.Method,
		"updated_at": p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *HTTPHandler) retryPayment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "payment_id required")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")

	// Try to read body for additional params (optional)
	body, _ := readBody(r)
	if body != nil {
		if key := strField(body, "idempotency_key"); key != "" {
			idempotencyKey = key
		}
	}

	p, err := h.uc.RetryPayment(r.Context(), id, idempotencyKey)
	if err != nil {
		log.Error().Err(err).Msg("retry payment failed")
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"payment_id": p.ID,
		"invoice_id": p.InvoiceID,
		"status":     string(p.Status),
		"amount":     p.Amount,
		"method":     p.Method,
		"qr_code":    p.QRCode,
	})
}

// --- Helpers ---

func readBody(r *http.Request) (map[string]interface{}, error) {
	defer r.Body.Close()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func strField(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"message": msg})
}
