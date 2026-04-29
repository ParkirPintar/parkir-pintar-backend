package model

type LocationUpdate struct {
	ReservationID string
	Latitude      float64
	Longitude     float64
}

type PresenceEvent struct {
	ReservationID string
	Event         string // LOCATION_UPDATED | WRONG_SPOT_DETECTED
	SpotID        string
}

// CheckInResult holds the result of a check-in operation.
type CheckInResult struct {
	ReservationID string
	Status        string // ACTIVE or BLOCKED
	CheckinAt     string
	WrongSpot     bool
}
