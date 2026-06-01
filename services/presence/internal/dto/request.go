package dto

// CheckInRequest holds the request body for check-in.
type CheckInRequest struct {
	ReservationID string `json:"reservation_id" binding:"required"`
	SpotID        string `json:"spot_id" binding:"required"`
}

// UpdateLocationRequest holds the request body for updating driver location.
type UpdateLocationRequest struct {
	ReservationID string  `json:"reservation_id" binding:"required"`
	Latitude      float64 `json:"latitude" binding:"required"`
	Longitude     float64 `json:"longitude" binding:"required"`
}

// CheckOutRequest holds the request body for check-out.
type CheckOutRequest struct {
	ReservationID string `json:"reservation_id" binding:"required"`
}
