// Package gateway reads the catalog and orders through the nginx gateway, for
// the reference cards a web consultation carries.
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/elug3/dupli1/shared/pkg/productclient"
	"github.com/elug3/dupli1/support/pkg/ports"
)

// requestTimeout bounds one lookup. A shopper is waiting on the send.
const requestTimeout = 5 * time.Second

func defaultClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return &http.Client{Timeout: requestTimeout}
}

// ProductReader looks variants up by SKU id, through product's variant
// endpoint — the one cart and order already use.
//
// Deliberately not GET /products/{id}: that route counts a guest view for any
// caller without product.read, and a server-side lookup would mint a new
// guest every time, inflating the PDP view counter.
type ProductReader struct {
	client *productclient.Client
}

func NewProductReader(baseURL string, client *http.Client) *ProductReader {
	return &ProductReader{client: productclient.NewClient(baseURL, defaultClient(client))}
}

func (r *ProductReader) Variant(ctx context.Context, skuID string) (*ports.ProductRef, error) {
	skuID = strings.TrimSpace(skuID)
	if skuID == "" {
		return nil, ports.ErrReferenceNotFound
	}
	variant, err := r.client.GetVariantBySkuID(ctx, url.PathEscape(skuID))
	if errors.Is(err, productclient.ErrVariantNotFound) {
		return nil, ports.ErrReferenceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("look up variant: %w", err)
	}
	return &ports.ProductRef{
		ProductID: variant.ProductID,
		SkuID:     variant.SkuID,
		SKU:       variant.SKU,
		Name:      variant.ProductName,
		Color:     variant.Color,
		PriceWon:  variant.UnitPriceWon,
		ImageURL:  variant.ImageURL,
	}, nil
}

// OrderReader reads one order with the caller's own bearer token, so order's
// ABAC — not this service — decides who may see it.
type OrderReader struct {
	baseURL string
	client  *http.Client
}

func NewOrderReader(baseURL string, client *http.Client) *OrderReader {
	return &OrderReader{baseURL: strings.TrimRight(baseURL, "/"), client: defaultClient(client)}
}

type orderResponse struct {
	ID       string    `json:"id"`
	Status   string    `json:"status"`
	TotalWon int64     `json:"total_won"`
	Created  time.Time `json:"created_at"`
	Items    []struct {
		ProductName string `json:"product_name"`
		SKU         string `json:"sku"`
		Quantity    int    `json:"quantity"`
	} `json:"items"`
}

func (r *OrderReader) Order(ctx context.Context, bearer, orderID string) (*ports.OrderRef, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" || strings.TrimSpace(bearer) == "" {
		return nil, ports.ErrReferenceNotFound
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		r.baseURL+"/api/v1/orders/"+url.PathEscape(orderID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("read order: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound, resp.StatusCode == http.StatusForbidden:
		// Someone else's order and no order at all answer the same, so a
		// shopper cannot probe for other people's order ids.
		return nil, ports.ErrReferenceNotFound
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, fmt.Errorf("read order: %s", resp.Status)
	}

	var body orderResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode order: %w", err)
	}
	ref := &ports.OrderRef{
		OrderID:   body.ID,
		Status:    body.Status,
		TotalWon:  body.TotalWon,
		ItemCount: len(body.Items),
		CreatedAt: body.Created,
	}
	if len(body.Items) > 0 {
		ref.FirstItemName = body.Items[0].ProductName
		if ref.FirstItemName == "" {
			ref.FirstItemName = body.Items[0].SKU
		}
	}
	return ref, nil
}
