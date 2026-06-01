package dto

// RetryPaymentRequest holds the request body for retrying a payment.
type RetryPaymentRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
}
