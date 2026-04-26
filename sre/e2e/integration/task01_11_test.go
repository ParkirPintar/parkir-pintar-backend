// Tasks 1–11: User Service — Auth flows
//   1. Register driver baru
//   2. Login → dapat JWT
//   3. Login credentials salah → 401
//   4. Akses endpoint tanpa token → 401
//   5. Akses endpoint dengan token expired → 401
//   6. Refresh token → dapat access token baru
//   7. Logout → token di-blacklist
//   8. Akses endpoint setelah logout → 401
//   9. Get profile driver
//  10. Update profile driver
//  11. Register duplicate license plate + vehicle type → 409

//go:build integration

package integration

import (
	"context"
	"testing"

	userpb "github.com/parkir-pintar/user/pkg/proto"
	searchpb "github.com/parkir-pintar/search/pkg/proto"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestTask01_RegisterDriver(t *testing.T) {
	conn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	plate := uniquePlate("T01")

	resp := &userpb.UserResponse{}
	err := conn.Invoke(context.Background(), "/proto.UserService/Register",
		&userpb.RegisterRequest{LicensePlate: plate, VehicleType: "CAR", Password: "test1234", Name: "Driver T01"}, resp)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if resp.UserId == "" {
		t.Fatal("expected non-empty user_id")
	}
	t.Logf("✓ Registered: user_id=%s plate=%s", resp.UserId, plate)
}

func TestTask02_LoginJWT(t *testing.T) {
	conn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	plate := uniquePlate("T02")

	// Register first
	conn.Invoke(context.Background(), "/proto.UserService/Register",
		&userpb.RegisterRequest{LicensePlate: plate, VehicleType: "CAR", Password: "test1234", Name: "Driver T02"},
		&userpb.UserResponse{})

	resp := &userpb.LoginResponse{}
	err := conn.Invoke(context.Background(), "/proto.UserService/Login",
		&userpb.LoginRequest{LicensePlate: plate, VehicleType: "CAR", Password: "test1234"}, resp)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatal("expected non-empty tokens")
	}
	t.Logf("✓ Login OK: access_token=%s... refresh_token=%s...", resp.AccessToken[:20], resp.RefreshToken[:20])
}

func TestTask03_LoginWrongCredentials(t *testing.T) {
	conn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	plate := uniquePlate("T03")

	conn.Invoke(context.Background(), "/proto.UserService/Register",
		&userpb.RegisterRequest{LicensePlate: plate, VehicleType: "CAR", Password: "test1234", Name: "Driver T03"},
		&userpb.UserResponse{})

	err := conn.Invoke(context.Background(), "/proto.UserService/Login",
		&userpb.LoginRequest{LicensePlate: plate, VehicleType: "CAR", Password: "wrongpass"},
		&userpb.LoginResponse{})
	if err == nil {
		t.Fatal("expected login to fail with wrong password")
	}

	st, _ := status.FromError(err)
	t.Logf("✓ Login rejected: code=%s message=%s", st.Code(), st.Message())
	// Unauthenticated = 16
	if st.Code() != 16 {
		t.Errorf("expected Unauthenticated (16), got %d", st.Code())
	}
}

func TestTask04_AccessWithoutToken(t *testing.T) {
	searchConn := dialGRPC(t, envOr("SEARCH_ADDR", "localhost:50055"))

	// Call search without JWT — should be rejected
	err := searchConn.Invoke(context.Background(), "/search.SearchService/GetAvailability",
		&searchpb.GetAvailabilityRequest{Floor: 1, VehicleType: "CAR"},
		&searchpb.GetAvailabilityResponse{})
	if err == nil {
		t.Fatal("expected error without token")
	}

	st, _ := status.FromError(err)
	t.Logf("✓ Access without token rejected: code=%s", st.Code())
	if st.Code() != 16 { // Unauthenticated
		t.Errorf("expected Unauthenticated (16), got %d", st.Code())
	}
}

func TestTask05_AccessWithExpiredToken(t *testing.T) {
	searchConn := dialGRPC(t, envOr("SEARCH_ADDR", "localhost:50055"))

	// Use a clearly expired/invalid JWT
	expiredToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0IiwiZXhwIjoxMDAwMDAwMDAwfQ.invalid"
	md := metadata.Pairs("authorization", "Bearer "+expiredToken)
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	err := searchConn.Invoke(ctx, "/search.SearchService/GetAvailability",
		&searchpb.GetAvailabilityRequest{Floor: 1, VehicleType: "CAR"},
		&searchpb.GetAvailabilityResponse{})
	if err == nil {
		t.Fatal("expected error with expired token")
	}

	st, _ := status.FromError(err)
	t.Logf("✓ Expired token rejected: code=%s", st.Code())
	if st.Code() != 16 { // Unauthenticated
		t.Errorf("expected Unauthenticated (16), got %d", st.Code())
	}
}

func TestTask06_RefreshToken(t *testing.T) {
	conn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	plate := uniquePlate("T06")

	conn.Invoke(context.Background(), "/proto.UserService/Register",
		&userpb.RegisterRequest{LicensePlate: plate, VehicleType: "CAR", Password: "test1234", Name: "Driver T06"},
		&userpb.UserResponse{})

	loginResp := &userpb.LoginResponse{}
	conn.Invoke(context.Background(), "/proto.UserService/Login",
		&userpb.LoginRequest{LicensePlate: plate, VehicleType: "CAR", Password: "test1234"}, loginResp)

	refreshResp := &userpb.LoginResponse{}
	err := conn.Invoke(context.Background(), "/proto.UserService/RefreshToken",
		&userpb.RefreshTokenRequest{RefreshToken: loginResp.RefreshToken}, refreshResp)
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	if refreshResp.AccessToken == "" {
		t.Fatal("expected new access token")
	}
	if refreshResp.AccessToken == loginResp.AccessToken {
		t.Error("expected different access token after refresh")
	}
	t.Logf("✓ Refresh OK: new access_token=%s...", refreshResp.AccessToken[:20])
}

func TestTask07_08_LogoutAndAccessAfter(t *testing.T) {
	userConn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	searchConn := dialGRPC(t, envOr("SEARCH_ADDR", "localhost:50055"))
	plate := uniquePlate("T07")

	userConn.Invoke(context.Background(), "/proto.UserService/Register",
		&userpb.RegisterRequest{LicensePlate: plate, VehicleType: "CAR", Password: "test1234", Name: "Driver T07"},
		&userpb.UserResponse{})

	loginResp := &userpb.LoginResponse{}
	userConn.Invoke(context.Background(), "/proto.UserService/Login",
		&userpb.LoginRequest{LicensePlate: plate, VehicleType: "CAR", Password: "test1234"}, loginResp)

	// Task 7: Logout
	logoutResp := &userpb.LogoutResponse{}
	md := metadata.Pairs("authorization", "Bearer "+loginResp.AccessToken)
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	err := userConn.Invoke(ctx, "/proto.UserService/Logout",
		&userpb.LogoutRequest{AccessToken: loginResp.AccessToken}, logoutResp)
	if err != nil {
		t.Fatalf("Logout failed: %v", err)
	}
	t.Log("✓ Task 7: Logout OK")

	// Task 8: Access endpoint after logout → should be rejected
	err = searchConn.Invoke(ctx, "/search.SearchService/GetAvailability",
		&searchpb.GetAvailabilityRequest{Floor: 1, VehicleType: "CAR"},
		&searchpb.GetAvailabilityResponse{})
	if err == nil {
		t.Fatal("expected error after logout")
	}

	st, _ := status.FromError(err)
	t.Logf("✓ Task 8: Access after logout rejected: code=%s", st.Code())
	if st.Code() != 16 { // Unauthenticated
		t.Errorf("expected Unauthenticated (16), got %d", st.Code())
	}
}

func TestTask09_GetProfile(t *testing.T) {
	conn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	plate := uniquePlate("T09")

	regResp := &userpb.UserResponse{}
	conn.Invoke(context.Background(), "/proto.UserService/Register",
		&userpb.RegisterRequest{LicensePlate: plate, VehicleType: "CAR", Password: "test1234", Name: "Driver T09"},
		regResp)

	loginResp := &userpb.LoginResponse{}
	conn.Invoke(context.Background(), "/proto.UserService/Login",
		&userpb.LoginRequest{LicensePlate: plate, VehicleType: "CAR", Password: "test1234"}, loginResp)

	md := metadata.Pairs("authorization", "Bearer "+loginResp.AccessToken)
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	profileResp := &userpb.UserResponse{}
	err := conn.Invoke(ctx, "/proto.UserService/GetProfile",
		&userpb.GetProfileRequest{UserId: loginResp.UserId}, profileResp)
	if err != nil {
		t.Fatalf("GetProfile failed: %v", err)
	}
	if profileResp.LicensePlate != plate {
		t.Errorf("expected plate=%s, got %s", plate, profileResp.LicensePlate)
	}
	t.Logf("✓ Profile: plate=%s name=%s", profileResp.LicensePlate, profileResp.Name)
}

func TestTask10_UpdateProfile(t *testing.T) {
	conn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	plate := uniquePlate("T10")

	regResp := &userpb.UserResponse{}
	conn.Invoke(context.Background(), "/proto.UserService/Register",
		&userpb.RegisterRequest{LicensePlate: plate, VehicleType: "CAR", Password: "test1234", Name: "Driver T10"},
		regResp)

	loginResp := &userpb.LoginResponse{}
	conn.Invoke(context.Background(), "/proto.UserService/Login",
		&userpb.LoginRequest{LicensePlate: plate, VehicleType: "CAR", Password: "test1234"}, loginResp)

	md := metadata.Pairs("authorization", "Bearer "+loginResp.AccessToken)
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	updateResp := &userpb.UserResponse{}
	err := conn.Invoke(ctx, "/proto.UserService/UpdateProfile",
		&userpb.UpdateProfileRequest{UserId: loginResp.UserId, Name: "Updated Name", PhoneNumber: "081234567890"}, updateResp)
	if err != nil {
		t.Fatalf("UpdateProfile failed: %v", err)
	}
	if updateResp.Name != "Updated Name" {
		t.Errorf("expected name='Updated Name', got '%s'", updateResp.Name)
	}
	t.Logf("✓ Profile updated: name=%s phone=%s", updateResp.Name, updateResp.PhoneNumber)
}

func TestTask11_RegisterDuplicate(t *testing.T) {
	conn := dialGRPC(t, envOr("USER_ADDR", "localhost:50051"))
	plate := uniquePlate("T11")

	// First registration — should succeed
	err := conn.Invoke(context.Background(), "/proto.UserService/Register",
		&userpb.RegisterRequest{LicensePlate: plate, VehicleType: "CAR", Password: "test1234", Name: "Driver T11"},
		&userpb.UserResponse{})
	if err != nil {
		t.Fatalf("First register failed: %v", err)
	}

	// Duplicate registration — should fail with AlreadyExists
	err = conn.Invoke(context.Background(), "/proto.UserService/Register",
		&userpb.RegisterRequest{LicensePlate: plate, VehicleType: "CAR", Password: "test1234", Name: "Driver T11 Dup"},
		&userpb.UserResponse{})
	if err == nil {
		t.Fatal("expected duplicate registration to fail")
	}

	st, _ := status.FromError(err)
	t.Logf("✓ Duplicate rejected: code=%s message=%s", st.Code(), st.Message())
	if st.Code() != 6 { // AlreadyExists
		t.Errorf("expected AlreadyExists (6), got %d", st.Code())
	}
}
