package interceptor

import (
	"context"
	"fmt"
	"strings"

	"github.com/parkir-pintar/user/internal/usecase"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// contextKey is an unexported type used for context value keys to avoid collisions.
type contextKey string

// driverIDKey is the context key for storing the authenticated driver ID.
const driverIDKey contextKey = "driver_id"

// DriverIDFromContext extracts the driver_id from the gRPC context.
// Returns the driver ID and true if present, or empty string and false otherwise.
func DriverIDFromContext(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(driverIDKey).(string)
	return val, ok
}

// UnaryAuthInterceptor returns a gRPC unary server interceptor that authenticates
// requests using JWT tokens from the "authorization" metadata header.
//
// It validates the JWT signature (HS256) and expiry, checks the token's jti against
// a Redis blacklist, and injects the driver_id into the gRPC context on success.
// Methods listed in skipMethods are allowed through without authentication.
func UnaryAuthInterceptor(jwtSecret string, redisClient *redis.Client, skipMethods []string) grpc.UnaryServerInterceptor {
	jwtHelper := usecase.NewJWTHelperWithSecret(jwtSecret)

	skip := make(map[string]struct{}, len(skipMethods))
	for _, m := range skipMethods {
		skip[m] = struct{}{}
	}

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// Skip authentication for whitelisted methods (e.g., Register, Login).
		if _, ok := skip[info.FullMethod]; ok {
			return handler(ctx, req)
		}

		// Extract the "authorization" value from gRPC metadata.
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		authValues := md.Get("authorization")
		if len(authValues) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization token")
		}

		tokenStr := authValues[0]
		// Strip "Bearer " prefix if present.
		tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")
		tokenStr = strings.TrimPrefix(tokenStr, "bearer ")
		if tokenStr == "" {
			return nil, status.Error(codes.Unauthenticated, "empty authorization token")
		}

		// Parse and validate JWT (HS256 signature + expiry).
		claims, err := jwtHelper.ParseAccessToken(tokenStr)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
		}

		// Validate that required claims are present.
		if claims.Subject == "" {
			return nil, status.Error(codes.Unauthenticated, "token missing subject claim")
		}
		if claims.ID == "" {
			return nil, status.Error(codes.Unauthenticated, "token missing jti claim")
		}

		// Check blacklist: Redis key "blacklist:{jti}".
		blacklisted, err := redisClient.Exists(ctx, fmt.Sprintf("blacklist:%s", claims.ID)).Result()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to check token blacklist: %v", err)
		}
		if blacklisted > 0 {
			return nil, status.Error(codes.Unauthenticated, "token has been revoked")
		}

		// Inject driver_id (from JWT sub claim) into context.
		ctx = context.WithValue(ctx, driverIDKey, claims.Subject)

		return handler(ctx, req)
	}
}
