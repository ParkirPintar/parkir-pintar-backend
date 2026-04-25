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
	b, err := h.uc.ChargeBookingFee(ctx, req.ReservationId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "charge booking fee: %v", err)
	}
	return &pb.BillingResponse{BillingId: b.ID, Status: string(b.Status)}, nil
}

func (h *BillingHandler) StartBillingSession(ctx context.Context, req *pb.StartBillingSessionRequest) (*pb.BillingResponse, error) {
	checkinAt, err := time.Parse(time.RFC3339, req.CheckinAt)
	if err != nil {
		checkinAt = time.Now()
	}
	if err := h.uc.StartSession(ctx, req.ReservationId, checkinAt); err != nil {
		return nil, status.Errorf(codes.Internal, "start session: %v", err)
	}
	return &pb.BillingResponse{Status: "OK"}, nil
}

func (h *BillingHandler) ApplyPenalty(ctx context.Context, req *pb.ApplyPenaltyRequest) (*pb.BillingResponse, error) {
	if err := h.uc.ApplyPenalty(ctx, req.ReservationId, req.Reason, req.Amount); err != nil {
		return nil, status.Errorf(codes.Internal, "apply penalty: %v", err)
	}
	return &pb.BillingResponse{Status: "OK"}, nil
}

func (h *BillingHandler) Checkout(ctx context.Context, req *pb.CheckoutRequest) (*pb.InvoiceResponse, error) {
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
		NoshowFee:       b.NoshowFee,
		CancellationFee: b.CancelFee,
		Total:           b.Total,
		Status:          string(b.Status),
	}, nil
}
