package model

type SpotStatus string

const (
	StatusAvailable SpotStatus = "AVAILABLE"
	StatusLocked    SpotStatus = "LOCKED"
	StatusReserved  SpotStatus = "RESERVED"
	StatusOccupied  SpotStatus = "OCCUPIED"
)

type Spot struct {
	SpotID      string     `db:"spot_id"`
	Floor       int        `db:"floor"`
	VehicleType string     `db:"vehicle_type"`
	Status      SpotStatus `db:"status"`
}
