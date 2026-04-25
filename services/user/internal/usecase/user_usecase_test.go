package usecase

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/parkir-pintar/user/internal/model"
	"golang.org/x/crypto/bcrypt"
)

// mockRepo implements repository.UserRepository for testing.
type mockRepo struct {
	users         map[string]*model.User          // keyed by ID
	byPlate       map[string]*model.User          // keyed by "plate|type"
	refreshTokens map[string]string               // token -> driverID
	blacklist     map[string]time.Duration         // jti -> ttl
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		users:         make(map[string]*model.User),
		byPlate:       make(map[string]*model.User),
		refreshTokens: make(map[string]string),
		blacklist:     make(map[string]time.Duration),
	}
}

func (m *mockRepo) Create(_ context.Context, u *model.User) error {
	m.users[u.ID] = u
	m.byPlate[u.LicensePlate+"|"+u.VehicleType] = u
	return nil
}

func (m *mockRepo) GetByID(_ context.Context, id string) (*model.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return u, nil
}

func (m *mockRepo) GetByLicensePlate(_ context.Context, plate, vtype string) (*model.User, error) {
	u, ok := m.byPlate[plate+"|"+vtype]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return u, nil
}

func (m *mockRepo) Update(_ context.Context, u *model.User) error {
	m.users[u.ID] = u
	m.byPlate[u.LicensePlate+"|"+u.VehicleType] = u
	return nil
}

func (m *mockRepo) SetTokenBlacklist(_ context.Context, jti string, ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("invalid TTL")
	}
	m.blacklist[jti] = ttl
	return nil
}

func (m *mockRepo) IsTokenBlacklisted(_ context.Context, jti string) (bool, error) {
	_, ok := m.blacklist[jti]
	return ok, nil
}

func (m *mockRepo) StoreRefreshToken(_ context.Context, token string, driverID string) error {
	m.refreshTokens[token] = driverID
	return nil
}

func (m *mockRepo) GetRefreshToken(_ context.Context, token string) (string, error) {
	id, ok := m.refreshTokens[token]
	if !ok {
		return "", fmt.Errorf("refresh token not found or expired")
	}
	return id, nil
}

func (m *mockRepo) DeleteRefreshToken(_ context.Context, token string) error {
	delete(m.refreshTokens, token)
	return nil
}

func setupUsecase() (UserUsecase, *mockRepo) {
	repo := newMockRepo()
	jwtHelper := NewJWTHelperWithSecret("test-secret-key")
	uc := NewUserUsecase(repo, jwtHelper)
	return uc, repo
}

func TestRegister_Success(t *testing.T) {
	uc, repo := setupUsecase()
	ctx := context.Background()

	user, err := uc.Register(ctx, "B1234XY", "CAR", "password123", "John", "08123456")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if user.LicensePlate != "B1234XY" {
		t.Errorf("expected license plate B1234XY, got %s", user.LicensePlate)
	}
	if user.VehicleType != "CAR" {
		t.Errorf("expected vehicle type CAR, got %s", user.VehicleType)
	}
	if user.Name != "John" {
		t.Errorf("expected name John, got %s", user.Name)
	}

	// Verify password is stored as bcrypt hash
	stored := repo.users[user.ID]
	if err := bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("password123")); err != nil {
		t.Errorf("password hash does not match: %v", err)
	}

	// Verify bcrypt cost is 12
	cost, err := bcrypt.Cost([]byte(stored.PasswordHash))
	if err != nil {
		t.Fatalf("failed to get bcrypt cost: %v", err)
	}
	if cost != 12 {
		t.Errorf("expected bcrypt cost 12, got %d", cost)
	}
}

func TestRegister_EmptyLicensePlate(t *testing.T) {
	uc, _ := setupUsecase()
	ctx := context.Background()

	_, err := uc.Register(ctx, "", "CAR", "password123", "John", "08123456")
	if err != ErrInvalidInput {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestRegister_EmptyVehicleType(t *testing.T) {
	uc, _ := setupUsecase()
	ctx := context.Background()

	_, err := uc.Register(ctx, "B1234XY", "", "password123", "John", "08123456")
	if err != ErrInvalidInput {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestRegister_DuplicateLicensePlate(t *testing.T) {
	uc, _ := setupUsecase()
	ctx := context.Background()

	_, err := uc.Register(ctx, "B1234XY", "CAR", "password123", "John", "08123456")
	if err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	_, err = uc.Register(ctx, "B1234XY", "CAR", "password456", "Jane", "08654321")
	if err != ErrDuplicateLicensePlate {
		t.Errorf("expected ErrDuplicateLicensePlate, got %v", err)
	}
}

func TestAuthenticate_Success(t *testing.T) {
	uc, _ := setupUsecase()
	ctx := context.Background()

	_, err := uc.Register(ctx, "B1234XY", "CAR", "password123", "John", "08123456")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	accessToken, refreshToken, err := uc.Authenticate(ctx, "B1234XY", "CAR", "password123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if accessToken == "" {
		t.Error("expected non-empty access token")
	}
	if refreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
}

func TestAuthenticate_WrongPassword(t *testing.T) {
	uc, _ := setupUsecase()
	ctx := context.Background()

	_, err := uc.Register(ctx, "B1234XY", "CAR", "password123", "John", "08123456")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, _, err = uc.Authenticate(ctx, "B1234XY", "CAR", "wrongpassword")
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthenticate_NonExistentUser(t *testing.T) {
	uc, _ := setupUsecase()
	ctx := context.Background()

	_, _, err := uc.Authenticate(ctx, "NONEXIST", "CAR", "password123")
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials (generic), got %v", err)
	}
}

func TestLogout_Success(t *testing.T) {
	uc, repo := setupUsecase()
	ctx := context.Background()

	_, err := uc.Register(ctx, "B1234XY", "CAR", "password123", "John", "08123456")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	accessToken, _, err := uc.Authenticate(ctx, "B1234XY", "CAR", "password123")
	if err != nil {
		t.Fatalf("authenticate failed: %v", err)
	}

	err = uc.Logout(ctx, accessToken)
	if err != nil {
		t.Fatalf("logout failed: %v", err)
	}

	// Verify jti is blacklisted
	jwtHelper := NewJWTHelperWithSecret("test-secret-key")
	claims, _ := jwtHelper.ParseAccessToken(accessToken)
	if _, ok := repo.blacklist[claims.ID]; !ok {
		t.Error("expected jti to be in blacklist")
	}
}

func TestLogout_BlacklistedTokenRejected(t *testing.T) {
	uc, _ := setupUsecase()
	ctx := context.Background()

	_, err := uc.Register(ctx, "B1234XY", "CAR", "password123", "John", "08123456")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	accessToken, _, err := uc.Authenticate(ctx, "B1234XY", "CAR", "password123")
	if err != nil {
		t.Fatalf("authenticate failed: %v", err)
	}

	err = uc.Logout(ctx, accessToken)
	if err != nil {
		t.Fatalf("logout failed: %v", err)
	}

	// ValidateToken should fail for blacklisted token
	_, err = uc.ValidateToken(ctx, accessToken)
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken for blacklisted token, got %v", err)
	}
}

func TestRefreshToken_Success(t *testing.T) {
	uc, repo := setupUsecase()
	ctx := context.Background()

	_, err := uc.Register(ctx, "B1234XY", "CAR", "password123", "John", "08123456")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, oldRefresh, err := uc.Authenticate(ctx, "B1234XY", "CAR", "password123")
	if err != nil {
		t.Fatalf("authenticate failed: %v", err)
	}

	newAccess, newRefresh, err := uc.RefreshToken(ctx, oldRefresh)
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	if newAccess == "" {
		t.Error("expected non-empty new access token")
	}
	if newRefresh == "" {
		t.Error("expected non-empty new refresh token")
	}
	if newRefresh == oldRefresh {
		t.Error("expected new refresh token to differ from old")
	}

	// Old refresh token should be deleted
	if _, ok := repo.refreshTokens[oldRefresh]; ok {
		t.Error("expected old refresh token to be deleted")
	}

	// New refresh token should exist
	if _, ok := repo.refreshTokens[newRefresh]; !ok {
		t.Error("expected new refresh token to be stored")
	}
}

func TestRefreshToken_InvalidToken(t *testing.T) {
	uc, _ := setupUsecase()
	ctx := context.Background()

	_, _, err := uc.RefreshToken(ctx, "nonexistent-token")
	if err != ErrRefreshTokenInvalid {
		t.Errorf("expected ErrRefreshTokenInvalid, got %v", err)
	}
}

func TestValidateToken_Success(t *testing.T) {
	uc, _ := setupUsecase()
	ctx := context.Background()

	user, err := uc.Register(ctx, "B1234XY", "CAR", "password123", "John", "08123456")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	accessToken, _, err := uc.Authenticate(ctx, "B1234XY", "CAR", "password123")
	if err != nil {
		t.Fatalf("authenticate failed: %v", err)
	}

	driverID, err := uc.ValidateToken(ctx, accessToken)
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if driverID != user.ID {
		t.Errorf("expected driver ID %s, got %s", user.ID, driverID)
	}
}

func TestValidateToken_InvalidToken(t *testing.T) {
	uc, _ := setupUsecase()
	ctx := context.Background()

	_, err := uc.ValidateToken(ctx, "invalid-token-string")
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestGetProfile_Success(t *testing.T) {
	uc, _ := setupUsecase()
	ctx := context.Background()

	user, err := uc.Register(ctx, "B1234XY", "CAR", "password123", "John", "08123456")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	profile, err := uc.GetProfile(ctx, user.ID)
	if err != nil {
		t.Fatalf("get profile failed: %v", err)
	}
	if profile.LicensePlate != "B1234XY" {
		t.Errorf("expected license plate B1234XY, got %s", profile.LicensePlate)
	}
	if profile.Name != "John" {
		t.Errorf("expected name John, got %s", profile.Name)
	}
}

func TestUpdateProfile_Success(t *testing.T) {
	uc, _ := setupUsecase()
	ctx := context.Background()

	user, err := uc.Register(ctx, "B1234XY", "CAR", "password123", "John", "08123456")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	updated, err := uc.UpdateProfile(ctx, user.ID, "Jane", "08999999")
	if err != nil {
		t.Fatalf("update profile failed: %v", err)
	}
	if updated.Name != "Jane" {
		t.Errorf("expected name Jane, got %s", updated.Name)
	}
	if updated.PhoneNumber != "08999999" {
		t.Errorf("expected phone 08999999, got %s", updated.PhoneNumber)
	}
	// License plate should remain unchanged
	if updated.LicensePlate != "B1234XY" {
		t.Errorf("expected license plate unchanged, got %s", updated.LicensePlate)
	}
}
