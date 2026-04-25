package interceptor

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const testSecret = "test-secret-key-for-auth-interceptor"

// generateTestToken creates a signed JWT for testing purposes.
func generateTestToken(t *testing.T, driverID, jti string, expiry time.Time) string {
	t.Helper()
	claims := jwt.RegisteredClaims{
		Subject:   driverID,
		ID:        jti,
		ExpiresAt: jwt.NewNumericDate(expiry),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return signed
}

// setupMiniredis creates a miniredis server and returns a connected redis.Client.
func setupMiniredis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return mr, rdb
}

// dummyHandler is a simple gRPC handler that returns "ok" for testing.
func dummyHandler(_ context.Context, _ any) (any, error) {
	return "ok", nil
}

func TestUnaryAuthInterceptor_SkipMethods(t *testing.T) {
	_, rdb := setupMiniredis(t)
	skipMethods := []string{
		"/user.UserService/Register",
		"/user.UserService/Login",
	}
	interceptor := UnaryAuthInterceptor(testSecret, rdb, skipMethods)

	// Register should be skipped — no token needed.
	info := &grpc.UnaryServerInfo{FullMethod: "/user.UserService/Register"}
	resp, err := interceptor(context.Background(), nil, info, dummyHandler)
	if err != nil {
		t.Fatalf("expected no error for skipped method Register, got: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("expected 'ok' response, got: %v", resp)
	}

	// Login should be skipped — no token needed.
	info = &grpc.UnaryServerInfo{FullMethod: "/user.UserService/Login"}
	resp, err = interceptor(context.Background(), nil, info, dummyHandler)
	if err != nil {
		t.Fatalf("expected no error for skipped method Login, got: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("expected 'ok' response, got: %v", resp)
	}
}

func TestUnaryAuthInterceptor_MissingMetadata(t *testing.T) {
	_, rdb := setupMiniredis(t)
	interceptor := UnaryAuthInterceptor(testSecret, rdb, nil)

	info := &grpc.UnaryServerInfo{FullMethod: "/user.UserService/GetProfile"}
	_, err := interceptor(context.Background(), nil, info, dummyHandler)
	if err == nil {
		t.Fatal("expected error for missing metadata")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.Unauthenticated {
		t.Fatalf("expected UNAUTHENTICATED, got: %v", err)
	}
}

func TestUnaryAuthInterceptor_MissingAuthorizationHeader(t *testing.T) {
	_, rdb := setupMiniredis(t)
	interceptor := UnaryAuthInterceptor(testSecret, rdb, nil)

	md := metadata.New(map[string]string{"other-key": "value"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{FullMethod: "/user.UserService/GetProfile"}
	_, err := interceptor(ctx, nil, info, dummyHandler)
	if err == nil {
		t.Fatal("expected error for missing authorization header")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.Unauthenticated {
		t.Fatalf("expected UNAUTHENTICATED, got: %v", err)
	}
}

func TestUnaryAuthInterceptor_InvalidToken(t *testing.T) {
	_, rdb := setupMiniredis(t)
	interceptor := UnaryAuthInterceptor(testSecret, rdb, nil)

	md := metadata.New(map[string]string{"authorization": "Bearer invalid-token-string"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{FullMethod: "/user.UserService/GetProfile"}
	_, err := interceptor(ctx, nil, info, dummyHandler)
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.Unauthenticated {
		t.Fatalf("expected UNAUTHENTICATED, got: %v", err)
	}
}

func TestUnaryAuthInterceptor_ExpiredToken(t *testing.T) {
	_, rdb := setupMiniredis(t)
	interceptor := UnaryAuthInterceptor(testSecret, rdb, nil)

	token := generateTestToken(t, "driver-123", uuid.NewString(), time.Now().Add(-1*time.Hour))
	md := metadata.New(map[string]string{"authorization": "Bearer " + token})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{FullMethod: "/user.UserService/GetProfile"}
	_, err := interceptor(ctx, nil, info, dummyHandler)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.Unauthenticated {
		t.Fatalf("expected UNAUTHENTICATED, got: %v", err)
	}
}

func TestUnaryAuthInterceptor_WrongSigningKey(t *testing.T) {
	_, rdb := setupMiniredis(t)
	interceptor := UnaryAuthInterceptor(testSecret, rdb, nil)

	claims := jwt.RegisteredClaims{
		Subject:   "driver-123",
		ID:        uuid.NewString(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte("wrong-secret"))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	md := metadata.New(map[string]string{"authorization": "Bearer " + signed})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{FullMethod: "/user.UserService/GetProfile"}
	_, err = interceptor(ctx, nil, info, dummyHandler)
	if err == nil {
		t.Fatal("expected error for wrong signing key")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.Unauthenticated {
		t.Fatalf("expected UNAUTHENTICATED, got: %v", err)
	}
}

func TestUnaryAuthInterceptor_BlacklistedToken(t *testing.T) {
	mr, rdb := setupMiniredis(t)
	interceptor := UnaryAuthInterceptor(testSecret, rdb, nil)

	jti := uuid.NewString()
	token := generateTestToken(t, "driver-456", jti, time.Now().Add(1*time.Hour))

	// Blacklist the token's jti in miniredis.
	mr.Set("blacklist:"+jti, "1")

	md := metadata.New(map[string]string{"authorization": "Bearer " + token})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{FullMethod: "/user.UserService/GetProfile"}
	_, err := interceptor(ctx, nil, info, dummyHandler)
	if err == nil {
		t.Fatal("expected error for blacklisted token")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.Unauthenticated {
		t.Fatalf("expected UNAUTHENTICATED, got: %v", err)
	}
}

func TestUnaryAuthInterceptor_ValidToken(t *testing.T) {
	_, rdb := setupMiniredis(t)
	interceptor := UnaryAuthInterceptor(testSecret, rdb, nil)

	driverID := "driver-789"
	jti := uuid.NewString()
	token := generateTestToken(t, driverID, jti, time.Now().Add(1*time.Hour))

	md := metadata.New(map[string]string{"authorization": "Bearer " + token})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var capturedDriverID string
	testHandler := func(ctx context.Context, _ any) (any, error) {
		id, ok := DriverIDFromContext(ctx)
		if !ok {
			t.Error("expected driver_id in context")
		}
		capturedDriverID = id
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/user.UserService/GetProfile"}
	resp, err := interceptor(ctx, nil, info, testHandler)
	if err != nil {
		t.Fatalf("expected no error for valid token, got: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("expected 'ok' response, got: %v", resp)
	}
	if capturedDriverID != driverID {
		t.Fatalf("expected driver_id=%s, got=%s", driverID, capturedDriverID)
	}
}

func TestUnaryAuthInterceptor_ValidTokenWithoutBearerPrefix(t *testing.T) {
	_, rdb := setupMiniredis(t)
	interceptor := UnaryAuthInterceptor(testSecret, rdb, nil)

	driverID := "driver-no-prefix"
	jti := uuid.NewString()
	token := generateTestToken(t, driverID, jti, time.Now().Add(1*time.Hour))

	// Send token without "Bearer " prefix.
	md := metadata.New(map[string]string{"authorization": token})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var capturedDriverID string
	testHandler := func(ctx context.Context, _ any) (any, error) {
		id, ok := DriverIDFromContext(ctx)
		if !ok {
			t.Error("expected driver_id in context")
		}
		capturedDriverID = id
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/user.UserService/GetProfile"}
	resp, err := interceptor(ctx, nil, info, testHandler)
	if err != nil {
		t.Fatalf("expected no error for valid token without prefix, got: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("expected 'ok' response, got: %v", resp)
	}
	if capturedDriverID != driverID {
		t.Fatalf("expected driver_id=%s, got=%s", driverID, capturedDriverID)
	}
}

func TestUnaryAuthInterceptor_NonSkippedMethodRequiresAuth(t *testing.T) {
	_, rdb := setupMiniredis(t)
	skipMethods := []string{
		"/user.UserService/Register",
		"/user.UserService/Login",
	}
	interceptor := UnaryAuthInterceptor(testSecret, rdb, skipMethods)

	// GetProfile is NOT in skipMethods, so it requires auth.
	info := &grpc.UnaryServerInfo{FullMethod: "/user.UserService/GetProfile"}
	_, err := interceptor(context.Background(), nil, info, dummyHandler)
	if err == nil {
		t.Fatal("expected error for non-skipped method without token")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.Unauthenticated {
		t.Fatalf("expected UNAUTHENTICATED, got: %v", err)
	}
}

func TestDriverIDFromContext_NotSet(t *testing.T) {
	id, ok := DriverIDFromContext(context.Background())
	if ok {
		t.Fatal("expected ok=false for empty context")
	}
	if id != "" {
		t.Fatalf("expected empty string, got: %s", id)
	}
}
