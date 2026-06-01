package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/parkir-pintar/presence/internal/dto"
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
	var req dto.CheckInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	result, err := h.uc.CheckIn(c.Request.Context(), req.ReservationID, req.SpotID)
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
	var req dto.UpdateLocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	event, err := h.uc.ProcessLocation(c.Request.Context(), model.LocationUpdate{
		ReservationID: req.ReservationID,
		Latitude:      req.Latitude,
		Longitude:     req.Longitude,
	})
	if err != nil {
		log.Error().Err(err).Msg("process location failed")
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	resp := gin.H{
		"reservation_id": req.ReservationID,
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
	var req dto.CheckOutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	if err := h.uc.CheckOut(c.Request.Context(), req.ReservationID); err != nil {
		log.Error().Err(err).Msg("check-out failed")
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"reservation_id": req.ReservationID,
		"status":         "CHECKOUT_INITIATED",
	})
}
