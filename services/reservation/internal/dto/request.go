package dto

// CreateReservationRequest holds the request body for creating a reservation.
type CreateReservationRequest struct {
	DriverID    string `json:"driver_id" binding:"required"`
	Mode        string `json:"mode" binding:"required,oneof=SYSTEM_ASSIGNED USER_SELECTED"`
	VehicleType string `json:"vehicle_type" binding:"required,oneof=CAR MOTORCYCLE"`
	SpotID      string `json:"spot_id"`
}

// HoldSpotRequest holds the request body for holding a spot.
type HoldSpotRequest struct {
	DriverID string `json:"driver_id" binding:"required"`
}
