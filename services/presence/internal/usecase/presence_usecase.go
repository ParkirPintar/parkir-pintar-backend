package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sync"

	"github.com/parkir-pintar/presence/internal/model"
	"github.com/rs/zerolog/log"
)

// PresenceUsecase processes real-time location updates and emits geofence events.
type PresenceUsecase interface {
	// ProcessLocation evaluates a location update against geofences and returns
	// a presence event if a geofence boundary was crossed, or nil otherwise.
	// streamID is used to track per-stream state for GEOFENCE_EXITED detection.
	ProcessLocation(ctx context.Context, streamID string, update model.LocationUpdate) (*model.PresenceEvent, error)

	// RemoveStream cleans up per-stream state when a stream ends.
	RemoveStream(streamID string)
}

// ReservationClient calls Reservation Service via gRPC.
type ReservationClient interface {
	CheckIn(ctx context.Context, reservationID, actualSpotID string) error
	GetReservation(ctx context.Context, reservationID string) (spotID string, err error)
}

// streamState tracks the last known spot for a given stream to detect exits.
type streamState struct {
	lastSpotID string
}

type presenceUsecase struct {
	reservation ReservationClient
	geofences   map[string]model.SpotGeofence // spotID -> geofence

	mu      sync.RWMutex
	streams map[string]*streamState // streamID -> state
}

// NewPresenceUsecase creates a PresenceUsecase with the given reservation client
// and geofence data loaded from the provided config path.
func NewPresenceUsecase(reservation ReservationClient, geofences map[string]model.SpotGeofence) PresenceUsecase {
	return &presenceUsecase{
		reservation: reservation,
		geofences:   geofences,
		streams:     make(map[string]*streamState),
	}
}

// LoadGeofences reads geofence configuration from a JSON file and returns
// a map of spotID -> SpotGeofence.
func LoadGeofences(path string) (map[string]model.SpotGeofence, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read geofences file: %w", err)
	}

	var entries []geofenceEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse geofences JSON: %w", err)
	}

	geofences := make(map[string]model.SpotGeofence, len(entries))
	for _, e := range entries {
		geofences[e.SpotID] = model.SpotGeofence{
			SpotID:    e.SpotID,
			Latitude:  e.Latitude,
			Longitude: e.Longitude,
			RadiusM:   e.RadiusM,
		}
	}

	log.Info().Int("count", len(geofences)).Msg("loaded geofence configurations")
	return geofences, nil
}

// geofenceEntry matches the JSON structure in configs/geofences.json.
type geofenceEntry struct {
	SpotID    string  `json:"spot_id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	RadiusM   float64 `json:"radius_m"`
}

func (u *presenceUsecase) ProcessLocation(ctx context.Context, streamID string, update model.LocationUpdate) (*model.PresenceEvent, error) {
	matchedSpot := u.findSpot(update.Latitude, update.Longitude)

	u.mu.Lock()
	state, exists := u.streams[streamID]
	if !exists {
		state = &streamState{}
		u.streams[streamID] = state
	}
	previousSpot := state.lastSpotID
	u.mu.Unlock()

	// Case 1: Driver was in a geofence and has now left it.
	if previousSpot != "" && matchedSpot != previousSpot {
		u.mu.Lock()
		state.lastSpotID = matchedSpot
		u.mu.Unlock()

		// If they moved to a different spot, we first emit GEOFENCE_EXITED for the old spot.
		// The next call will handle the new spot entry.
		if matchedSpot != "" {
			// Emit exit from old spot; the caller will get the entry event on the next update
			// or we can handle both. For simplicity, emit exit first.
			return &model.PresenceEvent{
				ReservationID: update.ReservationID,
				SpotID:        previousSpot,
				Event:         "GEOFENCE_EXITED",
			}, nil
		}

		// Driver left all geofences entirely.
		return &model.PresenceEvent{
			ReservationID: update.ReservationID,
			SpotID:        previousSpot,
			Event:         "GEOFENCE_EXITED",
		}, nil
	}

	// Case 2: Driver is not in any geofence (and wasn't before either).
	if matchedSpot == "" {
		return nil, nil
	}

	// Case 3: Driver is still in the same geofence — no event.
	if matchedSpot == previousSpot {
		return nil, nil
	}

	// Case 4: Driver just entered a geofence (previousSpot was "").
	u.mu.Lock()
	state.lastSpotID = matchedSpot
	u.mu.Unlock()

	// Look up the reserved spot for this reservation.
	reservedSpotID, err := u.reservation.GetReservation(ctx, update.ReservationID)
	if err != nil {
		log.Warn().Err(err).Str("reservation_id", update.ReservationID).Msg("failed to get reservation, emitting GEOFENCE_ENTERED only")
		return &model.PresenceEvent{
			ReservationID: update.ReservationID,
			SpotID:        matchedSpot,
			Event:         "GEOFENCE_ENTERED",
		}, nil
	}

	// Driver entered the correct reserved spot → trigger check-in.
	if matchedSpot == reservedSpotID {
		if err := u.reservation.CheckIn(ctx, update.ReservationID, matchedSpot); err != nil {
			log.Error().Err(err).Str("reservation_id", update.ReservationID).Msg("check-in failed")
			return &model.PresenceEvent{
				ReservationID: update.ReservationID,
				SpotID:        matchedSpot,
				Event:         "GEOFENCE_ENTERED",
			}, nil
		}

		return &model.PresenceEvent{
			ReservationID: update.ReservationID,
			SpotID:        matchedSpot,
			Event:         "CHECKIN_TRIGGERED",
		}, nil
	}

	// Driver entered a different spot than reserved → wrong spot.
	return &model.PresenceEvent{
		ReservationID: update.ReservationID,
		SpotID:        matchedSpot,
		Event:         "WRONG_SPOT_DETECTED",
	}, nil
}

// RemoveStream cleans up per-stream state when a stream ends.
func (u *presenceUsecase) RemoveStream(streamID string) {
	u.mu.Lock()
	delete(u.streams, streamID)
	u.mu.Unlock()
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

// haversineM calculates the distance in meters between two lat/lng points
// using the Haversine formula.
func haversineM(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000 // Earth radius in meters
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
