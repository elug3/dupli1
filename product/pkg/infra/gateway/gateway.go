// Package gateway reads orders and profiles through the nginx gateway for the
// review check, always with the shopper's own bearer token: order and profile
// decide what the shopper may see, not this service.
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/elug3/dupli1/product/pkg/ports"
)

// requestTimeout bounds one lookup. A shopper is waiting on the submit.
const requestTimeout = 5 * time.Second

func defaultClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return &http.Client{Timeout: requestTimeout}
}

// OrderReader reads GET /api/v1/orders/{id}.
type OrderReader struct {
	baseURL string
	client  *http.Client
}

func NewOrderReader(baseURL string, client *http.Client) *OrderReader {
	return &OrderReader{baseURL: strings.TrimRight(baseURL, "/"), client: defaultClient(client)}
}

type orderResponse struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	Status     string `json:"status"`
	Items      []struct {
		SkuID string `json:"sku_id"`
		SKU   string `json:"sku"`
	} `json:"items"`
}

func (r *OrderReader) Order(ctx context.Context, bearer, orderID string) (*ports.ReviewOrder, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" || strings.TrimSpace(bearer) == "" {
		return nil, ports.ErrOrderNotReadable
	}
	var body orderResponse
	status, err := getJSON(ctx, r.client, r.baseURL+"/api/v1/orders/"+url.PathEscape(orderID), bearer, &body)
	switch {
	case err != nil:
		return nil, fmt.Errorf("%w: read order: %v", ports.ErrReferenceUnavailable, err)
	case status == http.StatusNotFound, status == http.StatusForbidden:
		// Someone else's order and no order at all answer the same, so a
		// shopper cannot probe for other people's order ids.
		return nil, ports.ErrOrderNotReadable
	case status < 200 || status >= 300:
		return nil, fmt.Errorf("%w: read order: status %d", ports.ErrReferenceUnavailable, status)
	}
	out := &ports.ReviewOrder{ID: body.ID, CustomerID: body.CustomerID, Status: body.Status}
	for _, it := range body.Items {
		out.Lines = append(out.Lines, ports.OrderLine{SkuID: strings.TrimSpace(it.SkuID), SKU: strings.TrimSpace(it.SKU)})
	}
	return out, nil
}

// ProfileReader reads GET /api/v1/profile/me/profile.
type ProfileReader struct {
	baseURL string
	client  *http.Client
}

func NewProfileReader(baseURL string, client *http.Client) *ProfileReader {
	return &ProfileReader{baseURL: strings.TrimRight(baseURL, "/"), client: defaultClient(client)}
}

func (r *ProfileReader) DisplayName(ctx context.Context, bearer string) (string, error) {
	if strings.TrimSpace(bearer) == "" {
		return "", nil
	}
	var body struct {
		DisplayName string `json:"display_name"`
	}
	status, err := getJSON(ctx, r.client, r.baseURL+"/api/v1/profile/me/profile", bearer, &body)
	switch {
	case err != nil:
		return "", fmt.Errorf("%w: read profile: %v", ports.ErrReferenceUnavailable, err)
	case status == http.StatusNotFound:
		return "", nil
	case status < 200 || status >= 300:
		return "", fmt.Errorf("%w: read profile: status %d", ports.ErrReferenceUnavailable, status)
	}
	return body.DisplayName, nil
}

// getJSON decodes a 2xx body into out and returns the status.
func getJSON(ctx context.Context, client *http.Client, target, bearer string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return resp.StatusCode, fmt.Errorf("decode: %w", err)
	}
	return resp.StatusCode, nil
}
