package adapter

import (
	"context"
	"fmt"

	searchpb "github.com/parkir-pintar/search/pkg/proto"
	"google.golang.org/grpc"
)

// SearchClient abstracts calls to the Search Service gRPC API.
type SearchClient interface {
	GetFirstAvailable(ctx context.Context, vehicleType string) (spotID string, err error)
}

type searchClient struct {
	conn grpc.ClientConnInterface
}

// NewSearchClient creates a SearchClient backed by the given gRPC connection.
func NewSearchClient(conn grpc.ClientConnInterface) SearchClient {
	return &searchClient{conn: conn}
}

func (c *searchClient) GetFirstAvailable(ctx context.Context, vehicleType string) (string, error) {
	req := &searchpb.GetFirstAvailableRequest{
		VehicleType: vehicleType,
	}

	resp := &searchpb.GetFirstAvailableResponse{}
	err := c.conn.Invoke(ctx, "/search.SearchService/GetFirstAvailable", req, resp)
	if err != nil {
		return "", fmt.Errorf("search GetFirstAvailable: %w", err)
	}

	return resp.SpotId, nil
}
