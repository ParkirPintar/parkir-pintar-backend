package model

type LocationUpdate struct {
	ReservationID string
	Latitude      float64
	Longitude     float64
}

type PresenceEvent struct {
	ReservationID string
	Event         string // GEOFENCE_ENTERED | GEOFENCE_EXITED | CHECKIN_TRIGGERED | WRONG_SPOT_DETECTED
	SpotID        string
}

// SpotGeofence defines the center and radius of a parking spot geofence.
type SpotGeofence struct {
	SpotID    string
	Latitude  float64
	Longitude float64
	RadiusM   float64
}
