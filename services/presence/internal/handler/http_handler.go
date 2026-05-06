package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/parkir-pintar/presence/internal/model"
	"github.com/parkir-pintar/presence/internal/usecase"
	"github.com/rs/zerolog/log"
)

// HTTPHandler exposes REST endpoints for the Presence service.
type HTTPHandler struct {
	uc usecase.PresenceUsecase
}

// NewHTTPHandler creates a new HTTP handler with the given usecase.
func NewHTTPHandler(uc usecase.PresenceUsecase) *HTTPHandler {
	return &HTTPHandler{uc: uc}
}

// Register mounts all REST routes on the given mux.
func (h *HTTPHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/checkin", h.checkIn)
	mux.HandleFunc("POST /v1/presence/location", h.updateLocation)
	mux.HandleFunc("POST /v1/checkout/gate", h.checkOut)
}

func (h *HTTPHandler) checkIn(w http.ResponseWriter, r *http.Request) {
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
	spotID := strField(body, "spot_id")
	if spotID == "" {
		writeError(w, http.StatusBadRequest, "spot_id is required")
		return
	}

	result, err := h.uc.CheckIn(r.Context(), reservationID, spotID)
	if err != nil {
		if result != nil && result.WrongSpot {
			writeError(w, http.StatusConflict, "BLOCKED: must park at assigned spot")
			return
		}
		log.Error().Err(err).Msg("check-in failed")
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"reservation_id": result.ReservationID,
		"status":         result.Status,
		"checkin_at":     result.CheckinAt,
		"wrong_spot":     result.WrongSpot,
	})
}

func (h *HTTPHandler) updateLocation(w http.ResponseWriter, r *http.Request) {
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

	lat, _ := body["latitude"].(float64)
	lng, _ := body["longitude"].(float64)

	event, err := h.uc.ProcessLocation(r.Context(), model.LocationUpdate{
		ReservationID: reservationID,
		Latitude:      lat,
		Longitude:     lng,
	})
	if err != nil {
		log.Error().Err(err).Msg("process location failed")
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := map[string]interface{}{
		"reservation_id": reservationID,
		"event":          "NONE",
	}
	if event != nil {
		resp["event"] = event.Event
		if event.SpotID != "" {
			resp["spot_id"] = event.SpotID
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *HTTPHandler) checkOut(w http.ResponseWriter, r *http.Request) {
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

	if err := h.uc.CheckOut(r.Context(), reservationID); err != nil {
		log.Error().Err(err).Msg("check-out failed")
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"reservation_id": reservationID,
		"status":         "CHECKOUT_INITIATED",
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
