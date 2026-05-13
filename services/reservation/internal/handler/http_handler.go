package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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

// Register mounts all REST routes on the given Gin engine.
func (h *HTTPHandler) Register(r *gin.Engine) {
	r.POST("/v1/reservations", h.createReservation)
	r.GET("/v1/reservations/:id", h.getReservation)
	r.DELETE("/v1/reservations/:id", h.cancelReservation)
	r.POST("/v1/spots/:spot_id/hold", h.holdSpot)
}

func (h *HTTPHandler) createReservation(c *gin.Context) {
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}

	driverID := strField(body, "driver_id")
	if driverID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "driver_id is required"})
		return
	}
	mode := strField(body, "mode")
	if mode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "mode is required (SYSTEM_ASSIGNED or USER_SELECTED)"})
		return
	}
	if mode != "SYSTEM_ASSIGNED" && mode != "USER_SELECTED" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "mode must be SYSTEM_ASSIGNED or USER_SELECTED"})
		return
	}
	vehicleType := strField(body, "vehicle_type")
	if vehicleType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "vehicle_type is required"})
		return
	}
	if vehicleType != "CAR" && vehicleType != "MOTORCYCLE" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "vehicle_type must be CAR or MOTORCYCLE"})
		return
	}
	spotID := strField(body, "spot_id")
	if mode == "USER_SELECTED" && spotID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "spot_id is required for USER_SELECTED mode"})
		return
	}

	idempotencyKey := c.GetHeader("Idempotency-Key")

	res, err := h.uc.CreateReservation(c.Request.Context(), driverID, mode, vehicleType, spotID, idempotencyKey)
	if err != nil {
		if isFailedPreconditionHTTP(err) {
			c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
			return
		}
		log.Error().Err(err).Msg("create reservation failed")
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
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

func (h *HTTPHandler) getReservation(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "reservation_id required"})
		return
	}

	res, err := h.uc.GetReservation(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"reservation_id": res.ID,
		"spot_id":        res.SpotID,
		"mode":           string(res.Mode),
		"status":         string(res.Status),
		"booking_fee":    res.BookingFee,
		"confirmed_at":   formatTime(res.ConfirmedAt),
		"expires_at":     formatTime(res.ExpiresAt),
	})
}

func (h *HTTPHandler) cancelReservation(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "reservation_id required"})
		return
	}

	fee, err := h.uc.CancelReservation(c.Request.Context(), id)
	if err != nil {
		if isFailedPreconditionHTTP(err) {
			c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
			return
		}
		log.Error().Err(err).Msg("cancel reservation failed")
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"reservation_id":  id,
		"status":          "CANCELLED",
		"cancellation_fee": fee,
	})
}

func (h *HTTPHandler) holdSpot(c *gin.Context) {
	spotID := c.Param("spot_id")
	if spotID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "spot_id is required"})
		return
	}

	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}

	driverID := strField(body, "driver_id")
	if driverID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "driver_id is required"})
		return
	}

	heldUntil, err := h.uc.HoldSpot(c.Request.Context(), spotID, driverID)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"spot_id":    spotID,
		"held_until": heldUntil.Format(time.RFC3339),
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
