package handler

import (
	"context"

	"github.com/parkir-pintar/reservation/internal/usecase"
	pb "github.com/parkir-pintar/reservation/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ReservationHandler struct {
	pb.UnimplementedReservationServiceServer
	uc usecase.ReservationUsecase
}

func NewReservationHandler(uc usecase.ReservationUsecase) *ReservationHandler {
	return &ReservationHandler{uc: uc}
}

func (h *ReservationHandler) CreateReservation(ctx context.Context, req *pb.CreateReservationRequest) (*pb.ReservationResponse, error) {
	driverID := req.DriverId
	if driverID == "" {
		return nil, status.Error(codes.InvalidArgument, "driver_id is required")
	}
	if req.Mode == "" {
		return nil, status.Error(codes.InvalidArgument, "mode is required (SYSTEM_ASSIGNED or USER_SELECTED)")
	}
	if req.Mode != "SYSTEM_ASSIGNED" && req.Mode != "USER_SELECTED" {
		return nil, status.Error(codes.InvalidArgument, "mode must be SYSTEM_ASSIGNED or USER_SELECTED")
	}
	if req.VehicleType == "" {
		return nil, status.Error(codes.InvalidArgument, "vehicle_type is required")
	}
	if req.VehicleType != "CAR" && req.VehicleType != "MOTORCYCLE" {
		return nil, status.Error(codes.InvalidArgument, "vehicle_type must be CAR or MOTORCYCLE")
	}
	if req.Mode == "USER_SELECTED" && req.SpotId == "" {
		return nil, status.Error(codes.InvalidArgument, "spot_id is required for USER_SELECTED mode")
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
		PaymentId:     res.PaymentID,
		QrCode:        res.QRCode,
	}, nil
}

func (h *ReservationHandler) HoldSpot(ctx context.Context, req *pb.HoldSpotRequest) (*pb.HoldSpotResponse, error) {
	if req.SpotId == "" {
		return nil, status.Error(codes.InvalidArgument, "spot_id is required")
	}
	driverID := req.DriverId
	if driverID == "" {
		return nil, status.Error(codes.InvalidArgument, "driver_id is required")
	}

	heldUntil, err := h.uc.HoldSpot(ctx, req.SpotId, driverID)
	if err != nil {
		return nil, status.Errorf(codes.AlreadyExists, "spot held: %v", err)
	}
	return &pb.HoldSpotResponse{SpotId: req.SpotId, HeldUntil: heldUntil.String()}, nil
}

func (h *ReservationHandler) CancelReservation(ctx context.Context, req *pb.CancelReservationRequest) (*pb.CancelReservationResponse, error) {
	if req.ReservationId == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
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
	if req.ReservationId == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	if req.ActualSpotId == "" {
		return nil, status.Error(codes.InvalidArgument, "actual_spot_id is required")
	}
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
	if req.ReservationId == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
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
