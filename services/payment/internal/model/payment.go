package model

import "time"

type PaymentStatus string

const (
	PaymentPending PaymentStatus = "PENDING"
	PaymentPaid    PaymentStatus = "PAID"
	PaymentFailed  PaymentStatus = "FAILED"
)

type Payment struct {
	ID             string        `db:"id"`
	InvoiceID      string        `db:"invoice_id"`
	Amount         int64         `db:"amount"`
	Status         PaymentStatus `db:"status"`
	Method         string        `db:"method"`
	QRCode         string        `db:"qr_code"`
	IdempotencyKey string        `db:"idempotency_key"`
	UpdatedAt      time.Time     `db:"updated_at"`
}
