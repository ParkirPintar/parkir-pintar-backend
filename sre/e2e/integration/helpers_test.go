//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	reservationpb "github.com/parkir-pintar/reservation/pkg/proto"
	userpb "github.com/parkir-pintar/user/pkg/proto"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func dialGRPC(t *testing.T, addr string) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func newRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("REDIS_ADDR", "localhost:6379")})
	t.Cleanup(func() { rdb.Close() })
	return rdb
}

func connectDB(t *testing.T, urlKey, def string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), envOr(urlKey, def))
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}

// registerAndLogin creates a driver and returns a context with JWT metadata.
func registerAndLogin(t *testing.T, userConn *grpc.ClientConn, plate, vtype string) context.Context {
	t.Helper()
	ctx := context.Background()

	regReq := &userpb.RegisterRequest{
		LicensePlate: plate, VehicleType: vtype,
		Password: "test1234", Name: "Test Driver",
	}
	regResp := &userpb.UserResponse{}
	if err := userConn.Invoke(ctx, "/proto.UserService/Register", regReq, regResp); err != nil {
		t.Logf("register: %v (may already exist)", err)
	}

	loginReq := &userpb.LoginRequest{LicensePlate: plate, VehicleType: vtype, Password: "test1234"}
	loginResp := &userpb.LoginResponse{}
	if err := userConn.Invoke(ctx, "/proto.UserService/Login", loginReq, loginResp); err != nil {
		t.Fatalf("login failed: %v", err)
	}

	md := metadata.Pairs("authorization", "Bearer "+loginResp.AccessToken)
	return metadata.NewOutgoingContext(ctx, md)
}

// uniquePlate returns a unique license plate for test isolation.
func uniquePlate(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// createReservationAndWait creates a reservation and polls Redis until the queue worker processes it.
// Returns (reservationID, spotID).
func createReservationAndWait(t *testing.T, resConn *grpc.ClientConn, ctx context.Context, rdb *redis.Client, mode, vehicleType, spotID string) (string, string) {
	t.Helper()
	idemKey := fmt.Sprintf("idem-%d", time.Now().UnixNano())
	req := &reservationpb.CreateReservationRequest{
		Mode: mode, VehicleType: vehicleType, SpotId: spotID, IdempotencyKey: idemKey,
	}
	resp := &reservationpb.ReservationResponse{}
	if err := resConn.Invoke(ctx, "/reservation.ReservationService/CreateReservation", req, resp); err != nil {
		t.Fatalf("CreateReservation failed: %v", err)
	}

	// Poll for queue worker completion
	var reservationID string
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)
		id, err := rdb.Get(context.Background(), "idempotency:"+idemKey).Result()
		if err == nil && id != "" {
			reservationID = id
			break
		}
	}
	if reservationID == "" {
		t.Fatal("Queue worker did not process reservation within 10s")
	}

	// Get the actual spot assigned
	getResp := &reservationpb.ReservationResponse{}
	if err := resConn.Invoke(ctx, "/reservation.ReservationService/GetReservation",
		&reservationpb.GetReservationRequest{ReservationId: reservationID}, getResp); err != nil {
		t.Fatalf("GetReservation failed: %v", err)
	}

	return reservationID, getResp.SpotId
}
