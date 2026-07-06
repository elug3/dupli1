package httporder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/elug3/dupli1/payment/pkg/ports"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient}
}

func (c *Client) GetOrder(ctx context.Context, bearerToken, orderID string) (*ports.OrderSummary, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/orders/"+orderID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+bearerToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ports.ErrOrderNotFound
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, ports.ErrOrderForbidden
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("order request failed: %s", resp.Status)
	}

	var body struct {
		ID          string `json:"id"`
		CustomerID  string `json:"customer_id"`
		Status      string `json:"status"`
		TotalCents  int64  `json:"total_cents"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return &ports.OrderSummary{
		ID:         body.ID,
		CustomerID: body.CustomerID,
		Status:     body.Status,
		TotalCents: body.TotalCents,
	}, nil
}
