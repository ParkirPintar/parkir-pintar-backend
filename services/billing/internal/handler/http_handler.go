package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/parkir-pintar/billing/internal/usecase"
	"github.com/rs/zerolog/log"
)

// HTTPHandler exposes REST endpoints for the Billing service.
type HTTPHandler struct {
	uc usecase.BillingUsecase
}

// NewHTTPHandler creates a new HTTP handler with the given usecase.
func NewHTTPHandler(uc usecase.BillingUsecase) *HTTPHandler {
	return &HTTPHandler{uc: uc}
}

// Register mounts all REST routes on the given mux.
func (h *HTTPHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/checkout", h.checkout)
}

func (h *HTTPHandler) checkout(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	reservationID := strField(body, "reservation_id")
	if reservationID == "" {
		writeError(w, http.StatusBadRequest, "reservation_id is required")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")

	b, err := h.uc.Checkout(r.Context(), reservationID, idempotencyKey)
	if err != nil {
		log.Error().Err(err).Msg("checkout failed")
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"invoice_id":       b.ID,
		"reservation_id":   b.ReservationID,
		"booking_fee":      b.BookingFee,
		"hourly_fee":       b.HourlyFee,
		"overnight_fee":    b.OvernightFee,
		"penalty":          b.Penalty,
		"noshow_fee":       b.NoshowFee,
		"cancellation_fee": b.CancelFee,
		"total":            b.Total,
		"status":           string(b.Status),
		"qr_code":          b.QRCode,
		"payment_id":       b.PaymentID,
	})
}

// --- Helpers ---

func readBody(r *http.Request) (map[string]interface{}, error) {
	defer r.Body.Close()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
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
