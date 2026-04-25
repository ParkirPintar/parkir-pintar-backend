package usecase

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims represents the JWT access token claims.
type Claims struct {
	jwt.RegisteredClaims
	// sub = driver_id (in RegisteredClaims.Subject)
	// jti = uuid (in RegisteredClaims.ID)
	// exp = 1 hour from now (in RegisteredClaims.ExpiresAt)
}

// JWTHelper provides methods for generating and parsing JWT tokens.
type JWTHelper struct {
	signingKey []byte
}

// NewJWTHelper creates a new JWTHelper using the JWT_SECRET environment variable.
func NewJWTHelper() *JWTHelper {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "default-secret-change-me"
	}
	return &JWTHelper{signingKey: []byte(secret)}
}

// NewJWTHelperWithSecret creates a new JWTHelper with an explicit signing key.
// Useful for testing and dependency injection.
func NewJWTHelperWithSecret(secret string) *JWTHelper {
	return &JWTHelper{signingKey: []byte(secret)}
}

// GenerateAccessToken creates a signed HS256 JWT access token with 1-hour expiry.
// Claims include sub=driverID and jti=uuid.
func (h *JWTHelper) GenerateAccessToken(driverID string) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   driverID,
			ID:        uuid.NewString(),
			ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(h.signingKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}
	return signed, nil
}

// ParseAccessToken validates the token signature and expiry, then returns the claims.
func (h *JWTHelper) ParseAccessToken(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return h.signingKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

// GenerateRefreshToken returns an opaque UUID to be used as a refresh token.
func (h *JWTHelper) GenerateRefreshToken() string {
	return uuid.NewString()
}
