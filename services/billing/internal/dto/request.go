package dto

// CheckoutRequest holds the request body for checkout.
type CheckoutRequest struct {
	ReservationID string `json:"reservation_id" binding:"required"`
}
