package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/parkir-pintar/billing/internal/dto"
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

// Register mounts all REST routes on the given Gin engine.
func (h *HTTPHandler) Register(r *gin.Engine) {
	r.POST("/v1/checkout", h.checkout)
}

func (h *HTTPHandler) checkout(c *gin.Context) {
	var req dto.CheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	idempotencyKey := c.GetHeader("Idempotency-Key")

	b, err := h.uc.Checkout(c.Request.Context(), req.ReservationID, idempotencyKey)
	if err != nil {
		log.Error().Err(err).Msg("checkout failed")
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
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
