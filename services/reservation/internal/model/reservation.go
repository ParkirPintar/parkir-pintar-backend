package model

import "time"

type ReservationStatus string
type ReservationMode string

const (
	ModeSystemAssigned ReservationMode = "SYSTEM_ASSIGNED"
	ModeUserSelected   ReservationMode = "USER_SELECTED"

	StatusReserved  ReservationStatus = "RESERVED"
	StatusActive    ReservationStatus = "ACTIVE"
	StatusCompleted ReservationStatus = "COMPLETED"
	StatusCancelled ReservationStatus = "CANCELLED"
	StatusExpired   ReservationStatus = "EXPIRED"
)

type Reservation struct {
	ID             string            `db:"id"`
	DriverID       string            `db:"driver_id"`
	SpotID         string            `db:"spot_id"`
	Mode           ReservationMode   `db:"mode"`
	Status         ReservationStatus `db:"status"`
	BookingFee     int64             `db:"booking_fee"`
	ConfirmedAt    time.Time         `db:"confirmed_at"`
	ExpiresAt      time.Time         `db:"expires_at"`
	CheckinAt      *time.Time        `db:"checkin_at"`
	IdempotencyKey string            `db:"idempotency_key"`
	PaymentID      string            `db:"-"` // from billing, not persisted in reservation DB
	QRCode         string            `db:"-"` // from billing, not persisted in reservation DB
}
