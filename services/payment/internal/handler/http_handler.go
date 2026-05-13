package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
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

// Register mounts all REST routes on the given Gin engine.
func (h *HTTPHandler) Register(r *gin.Engine) {
	r.GET("/v1/payments/:id", h.getPaymentStatus)
	r.POST("/v1/payments/:id/retry", h.retryPayment)
}

func (h *HTTPHandler) getPaymentStatus(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "payment_id required"})
		return
	}

	// Strip /retry suffix if accidentally matched
	id = strings.TrimSuffix(id, "/retry")

	p, err := h.uc.GetPaymentStatus(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "payment not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"payment_id": p.ID,
		"invoice_id": p.InvoiceID,
		"status":     string(p.Status),
		"amount":     p.Amount,
		"method":     p.Method,
		"updated_at": p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *HTTPHandler) retryPayment(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "payment_id required"})
		return
	}

	idempotencyKey := c.GetHeader("Idempotency-Key")

	// Try to read body for additional params (optional)
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err == nil && body != nil {
		if key := strField(body, "idempotency_key"); key != "" {
			idempotencyKey = key
		}
	}

	p, err := h.uc.RetryPayment(c.Request.Context(), id, idempotencyKey)
	if err != nil {
		log.Error().Err(err).Msg("retry payment failed")
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"payment_id": p.ID,
		"invoice_id": p.InvoiceID,
		"status":     string(p.Status),
		"amount":     p.Amount,
		"method":     p.Method,
		"qr_code":    p.QRCode,
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
