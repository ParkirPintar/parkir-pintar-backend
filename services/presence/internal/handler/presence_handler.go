package handler

import (
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/parkir-pintar/presence/internal/model"
	"github.com/parkir-pintar/presence/internal/usecase"
	pb "github.com/parkir-pintar/presence/pkg/proto"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PresenceHandler implements the PresenceServiceServer gRPC interface.
type PresenceHandler struct {
	pb.UnimplementedPresenceServiceServer
	uc usecase.PresenceUsecase
}

// NewPresenceHandler creates a PresenceHandler with the given usecase.
func NewPresenceHandler(uc usecase.PresenceUsecase) *PresenceHandler {
	return &PresenceHandler{uc: uc}
}

// StreamLocation handles the bidirectional gRPC stream for real-time location updates.
func (h *PresenceHandler) StreamLocation(stream pb.PresenceService_StreamLocationServer) error {
	ctx := stream.Context()

	// Generate a unique stream ID for tracking per-stream geofence state.
	streamID := uuid.New().String()
	defer h.uc.RemoveStream(streamID)

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Internal, "recv: %v", err)
		}

		event, err := h.uc.ProcessLocation(ctx, streamID, model.LocationUpdate{
			ReservationID: req.ReservationId,
			Latitude:      req.Latitude,
			Longitude:     req.Longitude,
		})
		if err != nil {
			log.Error().Err(err).Str("reservation_id", req.ReservationId).Msg("process location error")
			continue
		}
		if event == nil {
			continue
		}

		if err := stream.Send(&pb.PresenceEvent{
			ReservationId: event.ReservationID,
			Event:         event.Event,
			SpotId:        event.SpotID,
			Timestamp:     time.Now().Format(time.RFC3339),
		}); err != nil {
			return status.Errorf(codes.Internal, "send: %v", err)
		}
	}
}
