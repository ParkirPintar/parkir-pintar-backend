package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/parkir-pintar/user/internal/model"
	"github.com/parkir-pintar/user/internal/usecase"
	pb "github.com/parkir-pintar/user/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockUserUsecase implements usecase.UserUsecase for testing.
type mockUserUsecase struct {
	registerFn     func(ctx context.Context, licensePlate, vehicleType, password, name, phoneNumber string) (*model.User, error)
	authenticateFn func(ctx context.Context, licensePlate, vehicleType, password string) (string, string, error)
	logoutFn       func(ctx context.Context, accessToken string) error
	refreshTokenFn func(ctx context.Context, refreshToken string) (string, string, error)
	validateFn     func(ctx context.Context, tokenStr string) (string, error)
	getProfileFn   func(ctx context.Context, driverID string) (*model.User, error)
	updateFn       func(ctx context.Context, driverID, name, phoneNumber string) (*model.User, error)
}

func (m *mockUserUsecase) Register(ctx context.Context, licensePlate, vehicleType, password, name, phoneNumber string) (*model.User, error) {
	return m.registerFn(ctx, licensePlate, vehicleType, password, name, phoneNumber)
}
func (m *mockUserUsecase) Authenticate(ctx context.Context, licensePlate, vehicleType, password string) (string, string, error) {
	return m.authenticateFn(ctx, licensePlate, vehicleType, password)
}
func (m *mockUserUsecase) Logout(ctx context.Context, accessToken string) error {
	return m.logoutFn(ctx, accessToken)
}
func (m *mockUserUsecase) RefreshToken(ctx context.Context, refreshToken string) (string, string, error) {
	return m.refreshTokenFn(ctx, refreshToken)
}
func (m *mockUserUsecase) ValidateToken(ctx context.Context, tokenStr string) (string, error) {
	return m.validateFn(ctx, tokenStr)
}
func (m *mockUserUsecase) GetProfile(ctx context.Context, driverID string) (*model.User, error) {
	return m.getProfileFn(ctx, driverID)
}
func (m *mockUserUsecase) UpdateProfile(ctx context.Context, driverID, name, phoneNumber string) (*model.User, error) {
	return m.updateFn(ctx, driverID, name, phoneNumber)
}

func TestRegister_Success(t *testing.T) {
	mock := &mockUserUsecase{
		registerFn: func(_ context.Context, lp, vt, pw, name, phone string) (*model.User, error) {
			return &model.User{ID: "u1", LicensePlate: lp, VehicleType: vt, Name: name, PhoneNumber: phone}, nil
		},
	}
	h := NewUserHandler(mock)
	resp, err := h.Register(context.Background(), &pb.RegisterRequest{
		LicensePlate: "B1234XY", VehicleType: "CAR", Password: "pass", Name: "John", PhoneNumber: "08123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.UserId != "u1" || resp.LicensePlate != "B1234XY" || resp.Name != "John" || resp.PhoneNumber != "08123" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestRegister_InvalidInput(t *testing.T) {
	mock := &mockUserUsecase{
		registerFn: func(_ context.Context, _, _, _, _, _ string) (*model.User, error) {
			return nil, usecase.ErrInvalidInput
		},
	}
	h := NewUserHandler(mock)
	_, err := h.Register(context.Background(), &pb.RegisterRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got: %v", st.Code())
	}
}

func TestRegister_DuplicateLicensePlate(t *testing.T) {
	mock := &mockUserUsecase{
		registerFn: func(_ context.Context, _, _, _, _, _ string) (*model.User, error) {
			return nil, usecase.ErrDuplicateLicensePlate
		},
	}
	h := NewUserHandler(mock)
	_, err := h.Register(context.Background(), &pb.RegisterRequest{LicensePlate: "B1234XY", VehicleType: "CAR"})
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.AlreadyExists {
		t.Fatalf("expected AlreadyExists, got: %v", st.Code())
	}
}

func TestLogin_Success(t *testing.T) {
	mock := &mockUserUsecase{
		authenticateFn: func(_ context.Context, _, _, _ string) (string, string, error) {
			return "access-tok", "refresh-tok", nil
		},
	}
	h := NewUserHandler(mock)
	resp, err := h.Login(context.Background(), &pb.LoginRequest{
		LicensePlate: "B1234XY", VehicleType: "CAR", Password: "pass",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AccessToken != "access-tok" || resp.RefreshToken != "refresh-tok" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestLogin_InvalidCredentials(t *testing.T) {
	mock := &mockUserUsecase{
		authenticateFn: func(_ context.Context, _, _, _ string) (string, string, error) {
			return "", "", usecase.ErrInvalidCredentials
		},
	}
	h := NewUserHandler(mock)
	_, err := h.Login(context.Background(), &pb.LoginRequest{LicensePlate: "B1234XY", VehicleType: "CAR", Password: "wrong"})
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got: %v", st.Code())
	}
}

func TestLogout_Success(t *testing.T) {
	mock := &mockUserUsecase{
		logoutFn: func(_ context.Context, _ string) error { return nil },
	}
	h := NewUserHandler(mock)
	resp, err := h.Logout(context.Background(), &pb.LogoutRequest{AccessToken: "some-token"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected success=true")
	}
}

func TestLogout_InvalidToken(t *testing.T) {
	mock := &mockUserUsecase{
		logoutFn: func(_ context.Context, _ string) error { return usecase.ErrInvalidToken },
	}
	h := NewUserHandler(mock)
	_, err := h.Logout(context.Background(), &pb.LogoutRequest{AccessToken: "bad-token"})
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got: %v", st.Code())
	}
}

func TestRefreshToken_Success(t *testing.T) {
	mock := &mockUserUsecase{
		refreshTokenFn: func(_ context.Context, _ string) (string, string, error) {
			return "new-access", "new-refresh", nil
		},
	}
	h := NewUserHandler(mock)
	resp, err := h.RefreshToken(context.Background(), &pb.RefreshTokenRequest{RefreshToken: "old-refresh"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AccessToken != "new-access" || resp.RefreshToken != "new-refresh" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestRefreshToken_Invalid(t *testing.T) {
	mock := &mockUserUsecase{
		refreshTokenFn: func(_ context.Context, _ string) (string, string, error) {
			return "", "", usecase.ErrRefreshTokenInvalid
		},
	}
	h := NewUserHandler(mock)
	_, err := h.RefreshToken(context.Background(), &pb.RefreshTokenRequest{RefreshToken: "expired"})
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got: %v", st.Code())
	}
}

func TestGetProfile_NotFound(t *testing.T) {
	mock := &mockUserUsecase{
		getProfileFn: func(_ context.Context, _ string) (*model.User, error) {
			return nil, errors.New("not found")
		},
	}
	h := NewUserHandler(mock)
	_, err := h.GetProfile(context.Background(), &pb.GetProfileRequest{UserId: "unknown"})
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Fatalf("expected Internal for generic error, got: %v", st.Code())
	}
}

func TestUpdateProfile_Success(t *testing.T) {
	mock := &mockUserUsecase{
		updateFn: func(_ context.Context, id, name, phone string) (*model.User, error) {
			return &model.User{ID: id, LicensePlate: "B1234XY", VehicleType: "CAR", Name: name, PhoneNumber: phone}, nil
		},
	}
	h := NewUserHandler(mock)
	resp, err := h.UpdateProfile(context.Background(), &pb.UpdateProfileRequest{
		UserId: "u1", Name: "Jane", PhoneNumber: "08999",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Name != "Jane" || resp.PhoneNumber != "08999" || resp.UserId != "u1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestMapError_InternalForUnknownError(t *testing.T) {
	err := mapError(errors.New("something unexpected"), "test")
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Fatalf("expected Internal, got: %v", st.Code())
	}
}
