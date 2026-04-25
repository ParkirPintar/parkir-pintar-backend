package model

import "time"

type BillingStatus string

const (
	BillingPending BillingStatus = "PENDING"
	BillingPaid    BillingStatus = "PAID"
	BillingFailed  BillingStatus = "FAILED"
)

type BillingRecord struct {
	ID            string        `db:"id"`
	ReservationID string        `db:"reservation_id"`
	BookingFee    int64         `db:"booking_fee"`
	HourlyFee     int64         `db:"hourly_fee"`
	OvernightFee  int64         `db:"overnight_fee"`
	Penalty       int64         `db:"penalty"`
	NoshowFee     int64         `db:"noshow_fee"`
	CancelFee     int64         `db:"cancellation_fee"`
	Total         int64         `db:"total"`
	Status        BillingStatus `db:"status"`
	SessionStart  *time.Time    `db:"session_start"`
	SessionEnd    *time.Time    `db:"session_end"`
}

type PricingInput struct {
	DurationHours         float64
	CrossesMidnight       bool
	WrongSpot             bool
	CancelElapsedMinutes  float64
	IsNoshow              bool
}

type PricingOutput struct {
	BookingFee      int64
	HourlyFee       int64
	OvernightFee    int64
	Penalty         int64
	CancellationFee int64
	Total           int64
}
