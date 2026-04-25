package model

import "time"

type TransactionEvent struct {
	ID            string    `db:"id"`
	EventType     string    `db:"event_type"`
	ReservationID string    `db:"reservation_id"`
	DriverID      string    `db:"driver_id"`
	SpotID        string    `db:"spot_id"`
	VehicleType   string    `db:"vehicle_type"`
	Amount        int64     `db:"amount"`
	Payload       []byte    `db:"payload"` // raw JSON
	RecordedAt    time.Time `db:"recorded_at"`
}
