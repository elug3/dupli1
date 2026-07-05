package httpproduct

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/elug3/dupli1/cart/pkg/ports"
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

type variantResponse struct {
	SKU       string   `json:"sku"`
	ProductID string   `json:"productId"`
	Color     string   `json:"color"`
	Price     float64  `json:"price"`
	Status    string   `json:"status"`
	ImageURLs []string `json:"imageUrls"`
}

func (c *Client) GetVariant(ctx context.Context, sku string) (*ports.VariantInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/variants/"+sku, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ports.ErrVariantNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("product request failed: %s", resp.Status)
	}

	var body variantResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	imageURL := ""
	if len(body.ImageURLs) > 0 {
		imageURL = body.ImageURLs[0]
	}

	return &ports.VariantInfo{
		SKU:            strings.ToUpper(strings.TrimSpace(body.SKU)),
		ProductID:      body.ProductID,
		Color:          body.Color,
		UnitPriceCents: int64(math.Round(body.Price * 100)),
		ImageURL:       imageURL,
	}, nil
}
