package usecase

import (
	"context"
	"math"

	"github.com/parkir-pintar/presence/internal/model"
)

type PresenceUsecase interface {
	ProcessLocation(ctx context.Context, update model.LocationUpdate) (*model.PresenceEvent, error)
}

// reservationClient calls Reservation Service via gRPC.
type reservationClient interface {
	CheckIn(ctx context.Context, reservationID, actualSpotID string) error
}

type presenceUsecase struct {
	reservation reservationClient
	geofences   map[string]model.SpotGeofence // spotID -> geofence; TODO: load from DB/config
}

func NewPresenceUsecase(reservation reservationClient) PresenceUsecase {
	return &presenceUsecase{
		reservation: reservation,
		geofences:   map[string]model.SpotGeofence{}, // TODO: populate from config/DB
	}
}

func (u *presenceUsecase) ProcessLocation(ctx context.Context, update model.LocationUpdate) (*model.PresenceEvent, error) {
	matchedSpot := u.findSpot(update.Latitude, update.Longitude)
	if matchedSpot == "" {
		return nil, nil
	}

	event := &model.PresenceEvent{
		ReservationID: update.ReservationID,
		SpotID:        matchedSpot,
		Event:         "GEOFENCE_ENTERED",
	}

	if err := u.reservation.CheckIn(ctx, update.ReservationID, matchedSpot); err != nil {
		event.Event = "WRONG_SPOT_DETECTED"
	} else {
		event.Event = "CHECKIN_TRIGGERED"
	}

	return event, nil
}

// findSpot returns the spotID whose geofence contains the given coordinates, or "".
func (u *presenceUsecase) findSpot(lat, lng float64) string {
	for spotID, gf := range u.geofences {
		if haversineM(lat, lng, gf.Latitude, gf.Longitude) <= gf.RadiusM {
			return spotID
		}
	}
	return ""
}

func haversineM(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
