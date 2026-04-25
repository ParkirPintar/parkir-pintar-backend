package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/parkir-pintar/notification/internal/model"
	"github.com/sony/gobreaker/v2"
)

// NotifProvider is the interface for the external notification stub (push/SMS).
type NotifProvider interface {
	Send(ctx context.Context, event model.NotificationEvent) error
}

type notifProvider struct {
	baseURL string
	http    *http.Client
	cb      *gobreaker.CircuitBreaker[[]byte]
}

// NewNotifProvider creates a NotifProvider that POSTs event JSON to the external
// notification provider stub at baseURL, wrapped with a gobreaker circuit breaker.
func NewNotifProvider(baseURL string) NotifProvider {
	cb := gobreaker.NewCircuitBreaker[[]byte](gobreaker.Settings{
		Name:        "notif-provider",
		MaxRequests: 3,
		Interval:    10 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	})
	return &notifProvider{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 5 * time.Second,
		},
		cb: cb,
	}
}

func (p *notifProvider) Send(ctx context.Context, event model.NotificationEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal notification event: %w", err)
	}

	_, err = p.cb.Execute(func() ([]byte, error) {
		url := p.baseURL + "/v1/notifications"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create notification request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := p.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("notification provider call: %w", err)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("notification provider returned %d: %s", resp.StatusCode, string(respBody))
		}

		return respBody, nil
	})

	return err
}
