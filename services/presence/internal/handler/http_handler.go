package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
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

// Register mounts all REST routes on the given Gin engine.
func (h *HTTPHandler) Register(r *gin.Engine) {
	r.POST("/v1/checkin", h.checkIn)
	r.POST("/v1/presence/location", h.updateLocation)
	r.POST("/v1/checkout/gate", h.checkOut)
}

func (h *HTTPHandler) checkIn(c *gin.Context) {
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}

	reservationID := strField(body, "reservation_id")
	if reservationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "reservation_id is required"})
		return
	}
	spotID := strField(body, "spot_id")
	if spotID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "spot_id is required"})
		return
	}

	result, err := h.uc.CheckIn(c.Request.Context(), reservationID, spotID)
	if err != nil {
		if result != nil && result.WrongSpot {
			c.JSON(http.StatusConflict, gin.H{"message": "BLOCKED: must park at assigned spot"})
			return
		}
		log.Error().Err(err).Msg("check-in failed")
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"reservation_id": result.ReservationID,
		"status":         result.Status,
		"checkin_at":     result.CheckinAt,
		"wrong_spot":     result.WrongSpot,
	})
}

func (h *HTTPHandler) updateLocation(c *gin.Context) {
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}

	reservationID := strField(body, "reservation_id")
	if reservationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "reservation_id is required"})
		return
	}

	lat, _ := body["latitude"].(float64)
	lng, _ := body["longitude"].(float64)

	event, err := h.uc.ProcessLocation(c.Request.Context(), model.LocationUpdate{
		ReservationID: reservationID,
		Latitude:      lat,
		Longitude:     lng,
	})
	if err != nil {
		log.Error().Err(err).Msg("process location failed")
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	resp := gin.H{
		"reservation_id": reservationID,
		"event":          "NONE",
	}
	if event != nil {
		resp["event"] = event.Event
		if event.SpotID != "" {
			resp["spot_id"] = event.SpotID
		}
	}

	c.JSON(http.StatusOK, resp)
}

func (h *HTTPHandler) checkOut(c *gin.Context) {
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}

	reservationID := strField(body, "reservation_id")
	if reservationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "reservation_id is required"})
		return
	}

	if err := h.uc.CheckOut(c.Request.Context(), reservationID); err != nil {
		log.Error().Err(err).Msg("check-out failed")
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"reservation_id": reservationID,
		"status":         "CHECKOUT_INITIATED",
	})
}

// --- Helpers ---

func strField(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
