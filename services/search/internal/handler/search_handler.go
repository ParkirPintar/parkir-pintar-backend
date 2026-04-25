package handler

import (
	"context"

	"github.com/parkir-pintar/search/internal/usecase"
	pb "github.com/parkir-pintar/search/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type SearchHandler struct {
	pb.UnimplementedSearchServiceServer
	uc usecase.SearchUsecase
}

func NewSearchHandler(uc usecase.SearchUsecase) *SearchHandler {
	return &SearchHandler{uc: uc}
}

func (h *SearchHandler) GetAvailability(ctx context.Context, req *pb.GetAvailabilityRequest) (*pb.GetAvailabilityResponse, error) {
	spots, err := h.uc.GetAvailability(ctx, int(req.Floor), req.VehicleType)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get availability: %v", err)
	}
	resp := &pb.GetAvailabilityResponse{
		Floor:          req.Floor,
		VehicleType:    req.VehicleType,
		TotalAvailable: int32(len(spots)),
	}
	for _, s := range spots {
		resp.Spots = append(resp.Spots, &pb.SpotSummary{
			SpotId:      s.SpotID,
			Floor:       int32(s.Floor),
			VehicleType: s.VehicleType,
			Status:      string(s.Status),
		})
	}
	return resp, nil
}

func (h *SearchHandler) GetFirstAvailable(ctx context.Context, req *pb.GetFirstAvailableRequest) (*pb.GetFirstAvailableResponse, error) {
	spot, err := h.uc.GetFirstAvailable(ctx, req.VehicleType)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "no available spot: %v", err)
	}
	return &pb.GetFirstAvailableResponse{SpotId: spot.SpotID}, nil
}
