package handler

import (
	"context"

	"github.com/parkir-pintar/reservation/internal/usecase"
	pb "github.com/parkir-pintar/reservation/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// driverIDKey matches the context key used by the auth interceptor.
type contextKey string

const driverIDKey contextKey = "driver_id"

// driverIDFromContext extracts the driver_id injected by the auth interceptor.
func driverIDFromContext(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(driverIDKey).(string)
	return val, ok
}

type ReservationHandler struct {
	pb.UnimplementedReservationServiceServer
	uc usecase.ReservationUsecase
}

func NewReservationHandler(uc usecase.ReservationUsecase) *ReservationHandler {
	return &ReservationHandler{uc: uc}
}

func (h *ReservationHandler) CreateReservation(ctx context.Context, req *pb.CreateReservationRequest) (*pb.ReservationResponse, error) {
	driverID, ok := driverIDFromContext(ctx)
	if !ok || driverID == "" {
		return nil, status.Error(codes.Unauthenticated, "driver_id not found in context")
	}

	res, err := h.uc.CreateReservation(ctx, driverID, req.Mode, req.VehicleType, req.SpotId, req.IdempotencyKey)
	if err != nil {
		if isFailedPrecondition(err) {
			return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
		}
		return nil, status.Errorf(codes.Unavailable, "create reservation: %v", err)
	}
	return &pb.ReservationResponse{
		ReservationId: res.ID,
		SpotId:        res.SpotID,
		Mode:          string(res.Mode),
		Status:        string(res.Status),
		BookingFee:    res.BookingFee,
		ConfirmedAt:   res.ConfirmedAt.String(),
		ExpiresAt:     res.ExpiresAt.String(),
	}, nil
}

func (h *ReservationHandler) HoldSpot(ctx context.Context, req *pb.HoldSpotRequest) (*pb.HoldSpotResponse, error) {
	driverID, ok := driverIDFromContext(ctx)
	if !ok || driverID == "" {
		return nil, status.Error(codes.Unauthenticated, "driver_id not found in context")
	}

	heldUntil, err := h.uc.HoldSpot(ctx, req.SpotId, driverID)
	if err != nil {
		return nil, status.Errorf(codes.AlreadyExists, "spot held: %v", err)
	}
	return &pb.HoldSpotResponse{SpotId: req.SpotId, HeldUntil: heldUntil.String()}, nil
}

func (h *ReservationHandler) CancelReservation(ctx context.Context, req *pb.CancelReservationRequest) (*pb.CancelReservationResponse, error) {
	fee, err := h.uc.CancelReservation(ctx, req.ReservationId)
	if err != nil {
		if isFailedPrecondition(err) {
			return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
		}
		return nil, status.Errorf(codes.Internal, "cancel: %v", err)
	}
	return &pb.CancelReservationResponse{
		ReservationId:   req.ReservationId,
		Status:          "CANCELLED",
		CancellationFee: fee,
	}, nil
}

func (h *ReservationHandler) CheckIn(ctx context.Context, req *pb.CheckInRequest) (*pb.CheckInResponse, error) {
	res, wrongSpot, penalty, err := h.uc.CheckIn(ctx, req.ReservationId, req.ActualSpotId)
	if err != nil {
		if isFailedPrecondition(err) {
			return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
		}
		return nil, status.Errorf(codes.Internal, "checkin: %v", err)
	}
	checkinAt := ""
	if res.CheckinAt != nil {
		checkinAt = res.CheckinAt.String()
	}
	return &pb.CheckInResponse{
		ReservationId:  req.ReservationId,
		Status:         "ACTIVE",
		CheckinAt:      checkinAt,
		WrongSpot:      wrongSpot,
		PenaltyApplied: penalty,
	}, nil
}

func (h *ReservationHandler) GetReservation(ctx context.Context, req *pb.GetReservationRequest) (*pb.ReservationResponse, error) {
	res, err := h.uc.GetReservation(ctx, req.ReservationId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "not found: %v", err)
	}
	return &pb.ReservationResponse{
		ReservationId: res.ID,
		SpotId:        res.SpotID,
		Mode:          string(res.Mode),
		Status:        string(res.Status),
		BookingFee:    res.BookingFee,
		ConfirmedAt:   res.ConfirmedAt.String(),
		ExpiresAt:     res.ExpiresAt.String(),
	}, nil
}

// isFailedPrecondition checks if the error message indicates a FAILED_PRECONDITION.
func isFailedPrecondition(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return len(msg) >= 21 && msg[:21] == "FAILED_PRECONDITION: " ||
		len(msg) >= 13 && msg[:13] == "HOLD_EXPIRED:"
}
