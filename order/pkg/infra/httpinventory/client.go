package httpinventory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/elug3/dupli1/order/pkg/ports"
)

type Client struct {
	baseURL     string
	httpClient  *http.Client
	bearerToken string
}

func NewClient(baseURL string, httpClient *http.Client, bearerToken string) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		httpClient:  httpClient,
		bearerToken: bearerToken,
	}
}

func (c *Client) Reserve(ctx context.Context, orderID string, items []ports.InventoryItem) (string, error) {
	var response struct {
		ReservationID string `json:"reservation_id"`
	}
	err := c.doJSON(ctx, http.MethodPost, "/api/v1/inventory/reservations", map[string]any{
		"order_id": orderID,
		"items":    items,
	}, &response)
	if err != nil {
		return "", err
	}
	if response.ReservationID == "" {
		return "", fmt.Errorf("inventory response missing reservation_id")
	}
	return response.ReservationID, nil
}

func (c *Client) CommitReservation(ctx context.Context, reservationID string) error {
	return c.doJSON(ctx, http.MethodPost, "/api/v1/inventory/reservations/"+reservationID+"/commit", nil, nil)
}

func (c *Client) ReleaseReservation(ctx context.Context, reservationID string) error {
	return c.doJSON(ctx, http.MethodPost, "/api/v1/inventory/reservations/"+reservationID+"/release", nil, nil)
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, target any) error {
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			return err
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, &payload)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		if errBody.Error == "" {
			errBody.Error = resp.Status
		}
		return fmt.Errorf("inventory request failed: %s", errBody.Error)
	}

	if target != nil {
		return json.NewDecoder(resp.Body).Decode(target)
	}
	return nil
}
