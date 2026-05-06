package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/parkir-pintar/reservation/internal/usecase"
	"github.com/rs/zerolog/log"
)

// HTTPHandler exposes REST endpoints for the Reservation service.
type HTTPHandler struct {
	uc usecase.ReservationUsecase
}

// NewHTTPHandler creates a new HTTP handler with the given usecase.
func NewHTTPHandler(uc usecase.ReservationUsecase) *HTTPHandler {
	return &HTTPHandler{uc: uc}
}

// Register mounts all REST routes on the given mux.
func (h *HTTPHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/reservations", h.createReservation)
	mux.HandleFunc("GET /v1/reservations/{id}", h.getReservation)
	mux.HandleFunc("DELETE /v1/reservations/{id}", h.cancelReservation)
	mux.HandleFunc("POST /v1/spots/{spot_id}/hold", h.holdSpot)
}

func (h *HTTPHandler) createReservation(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	driverID := strField(body, "driver_id")
	if driverID == "" {
		writeError(w, http.StatusBadRequest, "driver_id is required")
		return
	}
	mode := strField(body, "mode")
	if mode == "" {
		writeError(w, http.StatusBadRequest, "mode is required (SYSTEM_ASSIGNED or USER_SELECTED)")
		return
	}
	if mode != "SYSTEM_ASSIGNED" && mode != "USER_SELECTED" {
		writeError(w, http.StatusBadRequest, "mode must be SYSTEM_ASSIGNED or USER_SELECTED")
		return
	}
	vehicleType := strField(body, "vehicle_type")
	if vehicleType == "" {
		writeError(w, http.StatusBadRequest, "vehicle_type is required")
		return
	}
	if vehicleType != "CAR" && vehicleType != "MOTORCYCLE" {
		writeError(w, http.StatusBadRequest, "vehicle_type must be CAR or MOTORCYCLE")
		return
	}
	spotID := strField(body, "spot_id")
	if mode == "USER_SELECTED" && spotID == "" {
		writeError(w, http.StatusBadRequest, "spot_id is required for USER_SELECTED mode")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")

	res, err := h.uc.CreateReservation(r.Context(), driverID, mode, vehicleType, spotID, idempotencyKey)
	if err != nil {
		if isFailedPreconditionHTTP(err) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		log.Error().Err(err).Msg("create reservation failed")
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"reservation_id": res.ID,
		"spot_id":        res.SpotID,
		"mode":           string(res.Mode),
		"status":         string(res.Status),
		"booking_fee":    res.BookingFee,
		"confirmed_at":   formatTime(res.ConfirmedAt),
		"expires_at":     formatTime(res.ExpiresAt),
		"payment_id":     res.PaymentID,
		"qr_code":        res.QRCode,
	})
}

func (h *HTTPHandler) getReservation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "reservation_id required")
		return
	}

	res, err := h.uc.GetReservation(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"reservation_id": res.ID,
		"spot_id":        res.SpotID,
		"mode":           string(res.Mode),
		"status":         string(res.Status),
		"booking_fee":    res.BookingFee,
		"confirmed_at":   formatTime(res.ConfirmedAt),
		"expires_at":     formatTime(res.ExpiresAt),
	})
}

func (h *HTTPHandler) cancelReservation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "reservation_id required")
		return
	}

	fee, err := h.uc.CancelReservation(r.Context(), id)
	if err != nil {
		if isFailedPreconditionHTTP(err) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		log.Error().Err(err).Msg("cancel reservation failed")
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"reservation_id":  id,
		"status":          "CANCELLED",
		"cancellation_fee": fee,
	})
}

func (h *HTTPHandler) holdSpot(w http.ResponseWriter, r *http.Request) {
	spotID := r.PathValue("spot_id")
	if spotID == "" {
		writeError(w, http.StatusBadRequest, "spot_id is required")
		return
	}

	body, err := readBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	driverID := strField(body, "driver_id")
	if driverID == "" {
		writeError(w, http.StatusBadRequest, "driver_id is required")
		return
	}

	heldUntil, err := h.uc.HoldSpot(r.Context(), spotID, driverID)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"spot_id":    spotID,
		"held_until": heldUntil.Format(time.RFC3339),
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

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func isFailedPreconditionHTTP(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.HasPrefix(msg, "FAILED_PRECONDITION:") ||
		strings.HasPrefix(msg, "HOLD_EXPIRED:")
}
