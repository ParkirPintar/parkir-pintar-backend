package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
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

// Register mounts all REST routes on the given Gin engine.
func (h *HTTPHandler) Register(r *gin.Engine) {
	r.GET("/v1/availability", h.getAvailability)
	r.GET("/v1/availability/first", h.getFirstAvailable)
}

func (h *HTTPHandler) getAvailability(c *gin.Context) {
	vehicleType := c.Query("vehicle_type")
	if vehicleType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "vehicle_type is required"})
		return
	}
	if vehicleType != "CAR" && vehicleType != "MOTORCYCLE" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "vehicle_type must be CAR or MOTORCYCLE"})
		return
	}

	floorStr := c.Query("floor")
	floor, err := strconv.Atoi(floorStr)
	if err != nil || floor < 1 || floor > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "floor must be between 1 and 5"})
		return
	}

	spots, err := h.uc.GetAvailability(c.Request.Context(), floor, vehicleType)
	if err != nil {
		log.Error().Err(err).Msg("get availability failed")
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal error"})
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

	c.JSON(http.StatusOK, gin.H{
		"floor":           floor,
		"vehicle_type":    vehicleType,
		"total_available": len(spots),
		"spots":           spotsOut,
	})
}

func (h *HTTPHandler) getFirstAvailable(c *gin.Context) {
	vehicleType := c.Query("vehicle_type")
	if vehicleType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "vehicle_type is required"})
		return
	}
	if vehicleType != "CAR" && vehicleType != "MOTORCYCLE" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "vehicle_type must be CAR or MOTORCYCLE"})
		return
	}

	spot, err := h.uc.GetFirstAvailable(c.Request.Context(), vehicleType)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "no available spot"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"spot_id": spot.SpotID,
	})
}
