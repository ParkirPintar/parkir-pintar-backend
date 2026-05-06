package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/parkir-pintar/search/internal/usecase"
	"github.com/rs/zerolog/log"
)

// HTTPHandler exposes REST endpoints for the Search service.
type HTTPHandler struct {
	uc usecase.SearchUsecase
}

// NewHTTPHandler creates a new HTTP handler with the given usecase.
func NewHTTPHandler(uc usecase.SearchUsecase) *HTTPHandler {
	return &HTTPHandler{uc: uc}
}

// Register mounts all REST routes on the given mux.
func (h *HTTPHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/availability", h.getAvailability)
	mux.HandleFunc("GET /v1/availability/first", h.getFirstAvailable)
}

func (h *HTTPHandler) getAvailability(w http.ResponseWriter, r *http.Request) {
	vehicleType := r.URL.Query().Get("vehicle_type")
	if vehicleType == "" {
		writeError(w, http.StatusBadRequest, "vehicle_type is required")
		return
	}
	if vehicleType != "CAR" && vehicleType != "MOTORCYCLE" {
		writeError(w, http.StatusBadRequest, "vehicle_type must be CAR or MOTORCYCLE")
		return
	}

	floorStr := r.URL.Query().Get("floor")
	floor, err := strconv.Atoi(floorStr)
	if err != nil || floor < 1 || floor > 5 {
		writeError(w, http.StatusBadRequest, "floor must be between 1 and 5")
		return
	}

	spots, err := h.uc.GetAvailability(r.Context(), floor, vehicleType)
	if err != nil {
		log.Error().Err(err).Msg("get availability failed")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	type spotJSON struct {
		SpotID      string `json:"spot_id"`
		Floor       int    `json:"floor"`
		VehicleType string `json:"vehicle_type"`
		Status      string `json:"status"`
	}

	spotsOut := make([]spotJSON, 0, len(spots))
	for _, s := range spots {
		spotsOut = append(spotsOut, spotJSON{
			SpotID:      s.SpotID,
			Floor:       s.Floor,
			VehicleType: s.VehicleType,
			Status:      string(s.Status),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"floor":           floor,
		"vehicle_type":    vehicleType,
		"total_available": len(spots),
		"spots":           spotsOut,
	})
}

func (h *HTTPHandler) getFirstAvailable(w http.ResponseWriter, r *http.Request) {
	vehicleType := r.URL.Query().Get("vehicle_type")
	if vehicleType == "" {
		writeError(w, http.StatusBadRequest, "vehicle_type is required")
		return
	}
	if vehicleType != "CAR" && vehicleType != "MOTORCYCLE" {
		writeError(w, http.StatusBadRequest, "vehicle_type must be CAR or MOTORCYCLE")
		return
	}

	spot, err := h.uc.GetFirstAvailable(r.Context(), vehicleType)
	if err != nil {
		writeError(w, http.StatusNotFound, "no available spot")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"spot_id": spot.SpotID,
	})
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"message": msg})
}
