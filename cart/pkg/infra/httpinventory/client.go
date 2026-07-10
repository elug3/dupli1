package httpinventory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

type stockResponse struct {
	Quantity int `json:"quantity"`
	Reserved int `json:"reserved"`
}

func (c *Client) GetAvailableQty(ctx context.Context, sku string) (int, error) {
	return c.fetchAvailableQty(ctx, "/api/v1/inventory/"+sku)
}

func (c *Client) GetAvailableQtyBySkuID(ctx context.Context, skuID string) (int, error) {
	return c.fetchAvailableQty(ctx, "/api/v1/inventory/by-sku-id/"+skuID)
}

func (c *Client) fetchAvailableQty(ctx context.Context, path string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return 0, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return 0, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("inventory request failed: %s", resp.Status)
	}

	var body stockResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, err
	}
	available := body.Quantity - body.Reserved
	if available < 0 {
		return 0, nil
	}
	return available, nil
}
