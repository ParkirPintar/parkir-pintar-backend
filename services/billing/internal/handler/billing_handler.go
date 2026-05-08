package handler

import (
	"context"
	"time"

	"github.com/parkir-pintar/billing/internal/usecase"
	pb "github.com/parkir-pintar/billing/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type BillingHandler struct {
	pb.UnimplementedBillingServiceServer
	uc usecase.BillingUsecase
}

func NewBillingHandler(uc usecase.BillingUsecase) *BillingHandler {
	return &BillingHandler{uc: uc}
}

func (h *BillingHandler) ChargeBookingFee(ctx context.Context, req *pb.ChargeBookingFeeRequest) (*pb.BillingResponse, error) {
	if req.ReservationId == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	b, err := h.uc.ChargeBookingFee(ctx, req.ReservationId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "charge booking fee: %v", err)
	}
	return &pb.BillingResponse{BillingId: b.ID, Status: string(b.Status), PaymentId: b.PaymentID, QrCode: b.QRCode}, nil
}

func (h *BillingHandler) StartBillingSession(ctx context.Context, req *pb.StartBillingSessionRequest) (*pb.BillingResponse, error) {
	if req.ReservationId == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	if req.CheckinAt == "" {
		return nil, status.Error(codes.InvalidArgument, "checkin_at is required")
	}
	checkinAt, err := time.Parse(time.RFC3339, req.CheckinAt)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "checkin_at must be valid RFC3339: %v", err)
	}
	if err := h.uc.StartSession(ctx, req.ReservationId, checkinAt); err != nil {
		return nil, status.Errorf(codes.Internal, "start session: %v", err)
	}
	return &pb.BillingResponse{Status: "OK"}, nil
}

func (h *BillingHandler) ApplyPenalty(ctx context.Context, req *pb.ApplyPenaltyRequest) (*pb.BillingResponse, error) {
	if req.ReservationId == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	if req.Reason == "" {
		return nil, status.Error(codes.InvalidArgument, "reason is required")
	}
	if err := h.uc.ApplyPenalty(ctx, req.ReservationId, req.Reason, req.Amount); err != nil {
		return nil, status.Errorf(codes.Internal, "apply penalty: %v", err)
	}
	return &pb.BillingResponse{Status: "OK"}, nil
}

func (h *BillingHandler) Checkout(ctx context.Context, req *pb.CheckoutRequest) (*pb.InvoiceResponse, error) {
	if req.ReservationId == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	b, err := h.uc.Checkout(ctx, req.ReservationId, req.IdempotencyKey)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "checkout: %v", err)
	}
	return &pb.InvoiceResponse{
		InvoiceId:       b.ID,
		ReservationId:   b.ReservationID,
		BookingFee:      b.BookingFee,
		HourlyFee:       b.HourlyFee,
		OvernightFee:    b.OvernightFee,
		Penalty:         b.Penalty,
		NoshowFee:       0, // No-show has no additional fee — driver forfeits booking fee only
		CancellationFee: b.CancelFee,
		Total:           b.Total,
		Status:          string(b.Status),
		QrCode:          b.QRCode,
		PaymentId:       b.PaymentID,
	}, nil
}
