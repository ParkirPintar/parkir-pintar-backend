package handler

import (
	"context"

	"github.com/parkir-pintar/user/internal/usecase"
	pb "github.com/parkir-pintar/user/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UserHandler struct {
	pb.UnimplementedUserServiceServer
	uc usecase.UserUsecase
}

func NewUserHandler(uc usecase.UserUsecase) *UserHandler {
	return &UserHandler{uc: uc}
}

func (h *UserHandler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.UserResponse, error) {
	user, err := h.uc.Register(ctx, req.LicensePlate, req.VehicleType, req.Name, req.PhoneNumber)
	if err != nil {
		return nil, status.Errorf(codes.AlreadyExists, "register: %v", err)
	}
	return &pb.UserResponse{UserId: user.ID, LicensePlate: user.LicensePlate, VehicleType: user.VehicleType, Name: user.Name}, nil
}

func (h *UserHandler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	user, err := h.uc.Authenticate(ctx, req.LicensePlate, req.VehicleType)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "login: %v", err)
	}
	// TODO: issue JWT
	return &pb.LoginResponse{UserId: user.ID, AccessToken: "TODO", RefreshToken: "TODO"}, nil
}

func (h *UserHandler) GetProfile(ctx context.Context, req *pb.GetProfileRequest) (*pb.UserResponse, error) {
	user, err := h.uc.GetProfile(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "profile: %v", err)
	}
	return &pb.UserResponse{UserId: user.ID, LicensePlate: user.LicensePlate, VehicleType: user.VehicleType, Name: user.Name}, nil
}

func (h *UserHandler) UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.UserResponse, error) {
	user, err := h.uc.UpdateProfile(ctx, req.UserId, req.Name, req.PhoneNumber)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update profile: %v", err)
	}
	return &pb.UserResponse{UserId: user.ID, LicensePlate: user.LicensePlate, VehicleType: user.VehicleType, Name: user.Name}, nil
}
