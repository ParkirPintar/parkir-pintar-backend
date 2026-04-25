package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/parkir-pintar/user/internal/model"
	"github.com/parkir-pintar/user/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is the cost factor for bcrypt password hashing.
const bcryptCost = 12

// ErrInvalidCredentials is returned when authentication fails.
// It intentionally does not reveal whether the license plate exists.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrInvalidInput is returned when required fields are missing.
var ErrInvalidInput = errors.New("invalid input")

// ErrDuplicateLicensePlate is returned when a license plate + vehicle type already exists.
var ErrDuplicateLicensePlate = errors.New("duplicate license plate + vehicle type")

// ErrInvalidToken is returned when a token is invalid or blacklisted.
var ErrInvalidToken = errors.New("invalid token")

// ErrRefreshTokenInvalid is returned when a refresh token is not found or expired.
var ErrRefreshTokenInvalid = errors.New("refresh token not found or expired")

// UserUsecase defines the business logic interface for user operations.
type UserUsecase interface {
	Register(ctx context.Context, licensePlate, vehicleType, password, name, phoneNumber string) (*model.User, error)
	Authenticate(ctx context.Context, licensePlate, vehicleType, password string) (accessToken, refreshToken string, err error)
	Logout(ctx context.Context, accessToken string) error
	RefreshToken(ctx context.Context, refreshToken string) (newAccessToken, newRefreshToken string, err error)
	ValidateToken(ctx context.Context, tokenStr string) (driverID string, err error)
	GetProfile(ctx context.Context, driverID string) (*model.User, error)
	UpdateProfile(ctx context.Context, driverID, name, phoneNumber string) (*model.User, error)
}

type userUsecase struct {
	repo      repository.UserRepository
	jwtHelper *JWTHelper
}

// NewUserUsecase creates a new UserUsecase with the given repository and JWT helper.
func NewUserUsecase(repo repository.UserRepository, jwtHelper *JWTHelper) UserUsecase {
	return &userUsecase{repo: repo, jwtHelper: jwtHelper}
}

// Register creates a new driver with a bcrypt-hashed password.
func (u *userUsecase) Register(ctx context.Context, licensePlate, vehicleType, password, name, phoneNumber string) (*model.User, error) {
	if licensePlate == "" || vehicleType == "" {
		return nil, ErrInvalidInput
	}

	// Check for duplicate license plate + vehicle type
	if existing, _ := u.repo.GetByLicensePlate(ctx, licensePlate, vehicleType); existing != nil {
		return nil, ErrDuplicateLicensePlate
	}

	// Hash password with bcrypt cost 12
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user := &model.User{
		ID:           uuid.NewString(),
		LicensePlate: licensePlate,
		VehicleType:  vehicleType,
		PasswordHash: string(hashedPassword),
		Name:         name,
		PhoneNumber:  phoneNumber,
	}
	if err := u.repo.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// Authenticate validates credentials and returns JWT access + refresh tokens.
// Returns a generic "invalid credentials" error to avoid revealing whether the license plate exists.
func (u *userUsecase) Authenticate(ctx context.Context, licensePlate, vehicleType, password string) (string, string, error) {
	user, err := u.repo.GetByLicensePlate(ctx, licensePlate, vehicleType)
	if err != nil {
		// Do not reveal whether the license plate exists
		return "", "", ErrInvalidCredentials
	}

	// Compare password with stored bcrypt hash
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", "", ErrInvalidCredentials
	}

	// Generate JWT access token
	accessToken, err := u.jwtHelper.GenerateAccessToken(user.ID)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate access token: %w", err)
	}

	// Generate opaque refresh token and store in Redis
	refreshToken := u.jwtHelper.GenerateRefreshToken()
	if err := u.repo.StoreRefreshToken(ctx, refreshToken, user.ID); err != nil {
		return "", "", fmt.Errorf("failed to store refresh token: %w", err)
	}

	return accessToken, refreshToken, nil
}

// Logout parses the JWT, adds the jti to the Redis blacklist with remaining TTL,
// and deletes the associated refresh token.
func (u *userUsecase) Logout(ctx context.Context, accessToken string) error {
	claims, err := u.jwtHelper.ParseAccessToken(accessToken)
	if err != nil {
		return ErrInvalidToken
	}

	// Calculate remaining TTL from the token's exp claim
	remaining := time.Until(claims.ExpiresAt.Time)
	if remaining <= 0 {
		// Token already expired, nothing to blacklist
		return nil
	}

	// Add jti to blacklist with remaining TTL
	if err := u.repo.SetTokenBlacklist(ctx, claims.ID, remaining); err != nil {
		return fmt.Errorf("failed to blacklist token: %w", err)
	}

	return nil
}

// RefreshToken validates the refresh token in Redis, issues new access + refresh tokens,
// and deletes the old refresh token (rotation).
func (u *userUsecase) RefreshToken(ctx context.Context, refreshToken string) (string, string, error) {
	// Validate refresh token exists in Redis and get the associated driver ID
	driverID, err := u.repo.GetRefreshToken(ctx, refreshToken)
	if err != nil {
		return "", "", ErrRefreshTokenInvalid
	}

	// Delete old refresh token (rotation)
	if err := u.repo.DeleteRefreshToken(ctx, refreshToken); err != nil {
		return "", "", fmt.Errorf("failed to delete old refresh token: %w", err)
	}

	// Generate new access token
	newAccessToken, err := u.jwtHelper.GenerateAccessToken(driverID)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate new access token: %w", err)
	}

	// Generate new refresh token and store in Redis
	newRefreshToken := u.jwtHelper.GenerateRefreshToken()
	if err := u.repo.StoreRefreshToken(ctx, newRefreshToken, driverID); err != nil {
		return "", "", fmt.Errorf("failed to store new refresh token: %w", err)
	}

	return newAccessToken, newRefreshToken, nil
}

// ValidateToken parses the JWT, checks the blacklist, and returns the driver_id.
func (u *userUsecase) ValidateToken(ctx context.Context, tokenStr string) (string, error) {
	claims, err := u.jwtHelper.ParseAccessToken(tokenStr)
	if err != nil {
		return "", ErrInvalidToken
	}

	// Check if the token's jti is blacklisted
	blacklisted, err := u.repo.IsTokenBlacklisted(ctx, claims.ID)
	if err != nil {
		return "", fmt.Errorf("failed to check blacklist: %w", err)
	}
	if blacklisted {
		return "", ErrInvalidToken
	}

	return claims.Subject, nil
}

// GetProfile returns the driver profile by ID.
func (u *userUsecase) GetProfile(ctx context.Context, driverID string) (*model.User, error) {
	return u.repo.GetByID(ctx, driverID)
}

// UpdateProfile updates the driver's name and phone number.
func (u *userUsecase) UpdateProfile(ctx context.Context, driverID, name, phoneNumber string) (*model.User, error) {
	user, err := u.repo.GetByID(ctx, driverID)
	if err != nil {
		return nil, err
	}
	user.Name = name
	user.PhoneNumber = phoneNumber
	if err := u.repo.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}
