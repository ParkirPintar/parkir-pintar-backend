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
	driverID := "driver-from-jwt" // TODO: extract from gRPC metadata
	res, err := h.uc.CreateReservation(ctx, driverID, req.Mode, req.VehicleType, req.SpotId, req.IdempotencyKey)
	if err != nil {
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
	heldUntil, err := h.uc.HoldSpot(ctx, req.SpotId, req.DriverId)
	if err != nil {
		return nil, status.Errorf(codes.AlreadyExists, "spot held: %v", err)
	}
	return &pb.HoldSpotResponse{SpotId: req.SpotId, HeldUntil: heldUntil.String()}, nil
}

func (h *ReservationHandler) CancelReservation(ctx context.Context, req *pb.CancelReservationRequest) (*pb.CancelReservationResponse, error) {
	fee, err := h.uc.CancelReservation(ctx, req.ReservationId)
	if err != nil {
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
