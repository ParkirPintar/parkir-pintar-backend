package adapter

import (
	"context"
	"fmt"

	reservationpb "github.com/parkir-pintar/reservation/pkg/proto"
	"google.golang.org/grpc"
)

// ReservationClient abstracts calls to the Reservation Service gRPC API.
type ReservationClient interface {
	CheckIn(ctx context.Context, reservationID, actualSpotID string) error
	GetReservation(ctx context.Context, reservationID string) (spotID string, err error)
}

type reservationClient struct {
	conn grpc.ClientConnInterface
}

// NewReservationClient creates a ReservationClient backed by the given gRPC connection.
func NewReservationClient(conn grpc.ClientConnInterface) ReservationClient {
	return &reservationClient{conn: conn}
}

func (c *reservationClient) CheckIn(ctx context.Context, reservationID, actualSpotID string) error {
	req := &reservationpb.CheckInRequest{
		ReservationId: reservationID,
		ActualSpotId:  actualSpotID,
	}

	resp := &reservationpb.CheckInResponse{}
	err := c.conn.Invoke(ctx, "/reservation.ReservationService/CheckIn", req, resp)
	if err != nil {
		return fmt.Errorf("reservation CheckIn: %w", err)
	}

	return nil
}

func (c *reservationClient) GetReservation(ctx context.Context, reservationID string) (string, error) {
	req := &reservationpb.GetReservationRequest{
		ReservationId: reservationID,
	}

	resp := &reservationpb.ReservationResponse{}
	err := c.conn.Invoke(ctx, "/reservation.ReservationService/GetReservation", req, resp)
	if err != nil {
		return "", fmt.Errorf("reservation GetReservation: %w", err)
	}

	return resp.SpotId, nil
}
