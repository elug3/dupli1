package httpproduct

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/money"
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
	SkuID     string  `json:"skuId"`
	SKU       string  `json:"sku"`
	ProductID string  `json:"productId"`
	Price     float64 `json:"price"`
	Status    string  `json:"status"`
}

func (c *Client) GetVariant(ctx context.Context, sku string) (*ports.VariantInfo, error) {
	return c.fetchVariant(ctx, "/api/v1/products/variants/by-sku/"+sku)
}

func (c *Client) GetVariantBySkuID(ctx context.Context, skuID string) (*ports.VariantInfo, error) {
	return c.fetchVariant(ctx, "/api/v1/products/variants/by-sku-id/"+skuID)
}

func (c *Client) fetchVariant(ctx context.Context, path string) (*ports.VariantInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
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

	return &ports.VariantInfo{
		SkuID:          body.SkuID,
		SKU:            strings.ToUpper(strings.TrimSpace(body.SKU)),
		ProductID:      body.ProductID,
		UnitPriceCents: money.FromProductPrice(body.Price),
	}, nil
}
