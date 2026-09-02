// Package productclient is the shared HTTP client for product's public
// variant-lookup endpoint (GET /api/v1/products/variants/by-sku*), used by
// cart and order to resolve pricing/display data server-side.
//
// cart and order previously each carried a byte-identical ~85-line copy of
// this client, differing only in which one extra display field they mapped
// out of the response (cart: Color, order: ProductName) — both legitimate,
// different downstream needs, but everything around that one field
// (request construction, error handling, JSON decoding, SKU
// normalization, KRW-cents conversion) was pure duplication that had
// already started to drift in unrelated ways (one had gained a helper
// function the other hadn't).
package productclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/elug3/dupli1/shared/pkg/money"
)

// ErrVariantNotFound is returned when product has no such variant.
var ErrVariantNotFound = errors.New("variant not found")

// Variant is a superset of every field callers need from product's variant
// endpoint. Each service's own domain type picks only the fields it uses —
// this is infra-level plumbing, not a cross-service domain contract.
type Variant struct {
	SkuID       string
	SKU         string
	ProductID   string
	Color       string
	ProductName string
	// UnitPriceCents is whole KRW won (from product.price; not ×100).
	UnitPriceCents int64
	ImageURL       string
	// Status is decoded from the response but not currently read by any
	// caller.
	Status string
}

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
	SkuID       string   `json:"skuId"`
	SKU         string   `json:"sku"`
	ProductID   string   `json:"productId"`
	Color       string   `json:"color"`
	Price       float64  `json:"price"`
	Status      string   `json:"status"`
	ProductName string   `json:"productName"`
	ImageURLs   []string `json:"imageUrls"`
}

// GetVariant looks up a variant by SKU.
func (c *Client) GetVariant(ctx context.Context, sku string) (*Variant, error) {
	return c.fetchVariant(ctx, "/api/v1/products/variants/by-sku/"+sku)
}

// GetVariantBySkuID looks up a variant by its canonical SKU ID.
func (c *Client) GetVariantBySkuID(ctx context.Context, skuID string) (*Variant, error) {
	return c.fetchVariant(ctx, "/api/v1/products/variants/by-sku-id/"+skuID)
}

func (c *Client) fetchVariant(ctx context.Context, path string) (*Variant, error) {
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
		return nil, ErrVariantNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("product request failed: %s", resp.Status)
	}

	var body variantResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	return &Variant{
		SkuID:       body.SkuID,
		SKU:         strings.ToUpper(strings.TrimSpace(body.SKU)),
		ProductID:   body.ProductID,
		Color:       body.Color,
		ProductName: body.ProductName,
		// Product prices are KRW won; UnitPriceCents stores whole won (Stripe minor units for krw).
		UnitPriceCents: money.FromProductPrice(body.Price),
		ImageURL:       firstOf(body.ImageURLs),
		Status:         body.Status,
	}, nil
}

func firstOf(ss []string) string {
	if len(ss) > 0 {
		return ss[0]
	}
	return ""
}
