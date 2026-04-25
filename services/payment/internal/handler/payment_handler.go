package handler

import (
	"context"

	"github.com/parkir-pintar/payment/internal/usecase"
	pb "github.com/parkir-pintar/payment/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PaymentHandler struct {
	pb.UnimplementedPaymentServiceServer
	uc usecase.PaymentUsecase
}

func NewPaymentHandler(uc usecase.PaymentUsecase) *PaymentHandler {
	return &PaymentHandler{uc: uc}
}

func (h *PaymentHandler) CreatePayment(ctx context.Context, req *pb.CreatePaymentRequest) (*pb.PaymentResponse, error) {
	p, err := h.uc.CreatePayment(ctx, req.InvoiceId, req.Amount, req.IdempotencyKey)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create payment: %v", err)
	}
	return &pb.PaymentResponse{
		PaymentId: p.ID,
		InvoiceId: p.InvoiceID,
		Status:    string(p.Status),
		Amount:    p.Amount,
		Method:    p.Method,
		QrCode:    p.QRCode,
	}, nil
}

func (h *PaymentHandler) GetPaymentStatus(ctx context.Context, req *pb.GetPaymentStatusRequest) (*pb.PaymentResponse, error) {
	p, err := h.uc.GetPaymentStatus(ctx, req.PaymentId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "payment not found: %v", err)
	}
	return &pb.PaymentResponse{
		PaymentId: p.ID,
		InvoiceId: p.InvoiceID,
		Status:    string(p.Status),
		Amount:    p.Amount,
		Method:    p.Method,
		UpdatedAt: p.UpdatedAt.String(),
	}, nil
}

func (h *PaymentHandler) RetryPayment(ctx context.Context, req *pb.RetryPaymentRequest) (*pb.PaymentResponse, error) {
	p, err := h.uc.RetryPayment(ctx, req.PaymentId, req.IdempotencyKey)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "retry payment: %v", err)
	}
	return &pb.PaymentResponse{
		PaymentId: p.ID,
		InvoiceId: p.InvoiceID,
		Status:    string(p.Status),
		Amount:    p.Amount,
		Method:    p.Method,
		QrCode:    p.QRCode,
	}, nil
}
