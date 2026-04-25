package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/parkir-pintar/payment/internal/model"
)

// SettlementClient abstracts calls to the external settlement stub (Pondo Ngopi).
type SettlementClient interface {
	RequestQRIS(ctx context.Context, invoiceID string, amount int64) (string, error)
	CheckStatus(ctx context.Context, paymentID string) (model.PaymentStatus, error)
}

type settlementClient struct {
	baseURL string
	http    *http.Client
}

// NewSettlementClient creates a SettlementClient that calls the settlement stub at baseURL.
func NewSettlementClient(baseURL string) SettlementClient {
	return &settlementClient{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// qrisRequest is the JSON body sent to the settlement stub for QRIS creation.
type qrisRequest struct {
	InvoiceID string `json:"invoice_id"`
	Amount    int64  `json:"amount"`
}

// qrisResponse is the JSON body returned by the settlement stub for QRIS creation.
type qrisResponse struct {
	QRCode string `json:"qr_code"`
}

// statusResponse is the JSON body returned by the settlement stub for status checks.
type statusResponse struct {
	Status string `json:"status"`
}

func (c *settlementClient) RequestQRIS(ctx context.Context, invoiceID string, amount int64) (string, error) {
	body, err := json.Marshal(qrisRequest{
		InvoiceID: invoiceID,
		Amount:    amount,
	})
	if err != nil {
		return "", fmt.Errorf("marshal QRIS request: %w", err)
	}

	url := c.baseURL + "/v1/qris/create"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create QRIS request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("settlement QRIS call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("settlement QRIS returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result qrisResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode QRIS response: %w", err)
	}

	return result.QRCode, nil
}

func (c *settlementClient) CheckStatus(ctx context.Context, paymentID string) (model.PaymentStatus, error) {
	url := fmt.Sprintf("%s/v1/settlement/%s", c.baseURL, paymentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create status request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("settlement status call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("settlement status returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode status response: %w", err)
	}

	switch result.Status {
	case "PAID":
		return model.PaymentPaid, nil
	case "FAILED":
		return model.PaymentFailed, nil
	default:
		return model.PaymentPending, nil
	}
}
