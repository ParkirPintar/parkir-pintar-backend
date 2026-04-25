package handler

import (
	"context"
	"errors"

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
	user, err := h.uc.Register(ctx, req.LicensePlate, req.VehicleType, req.Password, req.Name, req.PhoneNumber)
	if err != nil {
		return nil, mapError(err, "register")
	}
	return &pb.UserResponse{
		UserId:       user.ID,
		LicensePlate: user.LicensePlate,
		VehicleType:  user.VehicleType,
		Name:         user.Name,
		PhoneNumber:  user.PhoneNumber,
	}, nil
}

func (h *UserHandler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	accessToken, refreshToken, err := h.uc.Authenticate(ctx, req.LicensePlate, req.VehicleType, req.Password)
	if err != nil {
		return nil, mapError(err, "login")
	}
	return &pb.LoginResponse{AccessToken: accessToken, RefreshToken: refreshToken}, nil
}

func (h *UserHandler) Logout(ctx context.Context, req *pb.LogoutRequest) (*pb.LogoutResponse, error) {
	if err := h.uc.Logout(ctx, req.AccessToken); err != nil {
		return nil, mapError(err, "logout")
	}
	return &pb.LogoutResponse{Success: true}, nil
}

func (h *UserHandler) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.LoginResponse, error) {
	accessToken, refreshToken, err := h.uc.RefreshToken(ctx, req.RefreshToken)
	if err != nil {
		return nil, mapError(err, "refresh token")
	}
	return &pb.LoginResponse{AccessToken: accessToken, RefreshToken: refreshToken}, nil
}

func (h *UserHandler) GetProfile(ctx context.Context, req *pb.GetProfileRequest) (*pb.UserResponse, error) {
	user, err := h.uc.GetProfile(ctx, req.UserId)
	if err != nil {
		return nil, mapError(err, "profile")
	}
	return &pb.UserResponse{
		UserId:       user.ID,
		LicensePlate: user.LicensePlate,
		VehicleType:  user.VehicleType,
		Name:         user.Name,
		PhoneNumber:  user.PhoneNumber,
	}, nil
}

func (h *UserHandler) UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.UserResponse, error) {
	user, err := h.uc.UpdateProfile(ctx, req.UserId, req.Name, req.PhoneNumber)
	if err != nil {
		return nil, mapError(err, "update profile")
	}
	return &pb.UserResponse{
		UserId:       user.ID,
		LicensePlate: user.LicensePlate,
		VehicleType:  user.VehicleType,
		Name:         user.Name,
		PhoneNumber:  user.PhoneNumber,
	}, nil
}

// mapError converts usecase errors to appropriate gRPC status codes.
func mapError(err error, op string) error {
	switch {
	case errors.Is(err, usecase.ErrInvalidInput):
		return status.Errorf(codes.InvalidArgument, "%s: %v", op, err)
	case errors.Is(err, usecase.ErrDuplicateLicensePlate):
		return status.Errorf(codes.AlreadyExists, "%s: %v", op, err)
	case errors.Is(err, usecase.ErrInvalidCredentials):
		return status.Errorf(codes.Unauthenticated, "%s: %v", op, err)
	case errors.Is(err, usecase.ErrInvalidToken):
		return status.Errorf(codes.Unauthenticated, "%s: %v", op, err)
	case errors.Is(err, usecase.ErrRefreshTokenInvalid):
		return status.Errorf(codes.Unauthenticated, "%s: %v", op, err)
	default:
		return status.Errorf(codes.Internal, "%s: %v", op, err)
	}
}
