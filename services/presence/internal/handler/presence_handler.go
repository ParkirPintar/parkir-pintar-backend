package handler

import (
	"context"
	"time"

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

// UpdateLocation handles a single location update from the driver app.
// Called every ≤30 seconds while session is active.
func (h *PresenceHandler) UpdateLocation(ctx context.Context, req *pb.LocationUpdate) (*pb.PresenceEvent, error) {
	if req.ReservationId == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}

	event, err := h.uc.ProcessLocation(ctx, model.LocationUpdate{
		ReservationID: req.ReservationId,
		Latitude:      req.Latitude,
		Longitude:     req.Longitude,
	})
	if err != nil {
		log.Error().Err(err).Str("reservation_id", req.ReservationId).Msg("process location error")
		return nil, status.Errorf(codes.Internal, "process location: %v", err)
	}

	if event == nil {
		return &pb.PresenceEvent{
			ReservationId: req.ReservationId,
			Event:         "NONE",
			Timestamp:     time.Now().Format(time.RFC3339),
		}, nil
	}

	return &pb.PresenceEvent{
		ReservationId: event.ReservationID,
		Event:         event.Event,
		SpotId:        event.SpotID,
		Timestamp:     time.Now().Format(time.RFC3339),
	}, nil
}

// CheckIn handles the check-in request when the driver enters the parking gate.
// Presence validates the spot, calls Reservation.CheckIn, then calls
// Billing.StartBillingSession to start the billing timer.
func (h *PresenceHandler) CheckIn(ctx context.Context, req *pb.CheckInRequest) (*pb.CheckInResponse, error) {
	if req.ReservationId == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	if req.SpotId == "" {
		return nil, status.Error(codes.InvalidArgument, "spot_id is required")
	}

	result, err := h.uc.CheckIn(ctx, req.ReservationId, req.SpotId)
	if err != nil {
		if result != nil && result.WrongSpot {
			return nil, status.Errorf(codes.FailedPrecondition, "BLOCKED: must park at assigned spot")
		}
		return nil, status.Errorf(codes.Internal, "check-in: %v", err)
	}

	return &pb.CheckInResponse{
		ReservationId: result.ReservationID,
		Status:        result.Status,
		CheckinAt:     result.CheckinAt,
		WrongSpot:     result.WrongSpot,
	}, nil
}

// CheckOut handles the check-out request when the driver wants to leave.
func (h *PresenceHandler) CheckOut(ctx context.Context, req *pb.CheckOutRequest) (*pb.CheckOutResponse, error) {
	if req.ReservationId == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}

	err := h.uc.CheckOut(ctx, req.ReservationId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "check-out: %v", err)
	}

	return &pb.CheckOutResponse{
		ReservationId: req.ReservationId,
		Status:        "CHECKOUT_INITIATED",
	}, nil
}
