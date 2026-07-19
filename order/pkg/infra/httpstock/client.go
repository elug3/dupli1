package httpstock

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/elug3/dupli1/order/pkg/infra/httpauth"
	"github.com/elug3/dupli1/order/pkg/ports"
)

// ErrUnauthorized is returned when product rejects the service-account Bearer token.
var ErrUnauthorized = errors.New("unauthorized")

// Client calls product stock/reservation APIs.
type Client struct {
	baseURL     string
	httpClient  *http.Client
	tokenSource httpauth.TokenSource
}

func NewClient(baseURL string, httpClient *http.Client, tokenSource httpauth.TokenSource) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		httpClient:  httpClient,
		tokenSource: tokenSource,
	}
}

// NewClientWithBearer builds a client with a fixed bearer token (tests / static override).
func NewClientWithBearer(baseURL string, httpClient *http.Client, bearerToken string) *Client {
	var src httpauth.TokenSource
	if bearerToken != "" {
		src = httpauth.StaticToken(bearerToken)
	}
	return NewClient(baseURL, httpClient, src)
}

func (c *Client) Reserve(ctx context.Context, orderID string, items []ports.StockItem) (string, error) {
	var response struct {
		ReservationID string `json:"reservation_id"`
	}
	// Product still serves stock under /api/v1/inventory/* (legacy path prefix).
	err := c.doJSON(ctx, http.MethodPost, "/api/v1/inventory/reservations", map[string]any{
		"order_id": orderID,
		"items":    items,
	}, &response)
	if err != nil {
		return "", err
	}
	if response.ReservationID == "" {
		return "", fmt.Errorf("stock response missing reservation_id")
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
	err := c.doJSONOnce(ctx, method, path, body, target)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrUnauthorized) {
		return err
	}
	// Stale access token — invalidate and retry once with a fresh token.
	if inv, ok := c.tokenSource.(interface{ Invalidate() }); ok {
		inv.Invalidate()
	}
	return c.doJSONOnce(ctx, method, path, body, target)
}

func (c *Client) doJSONOnce(ctx context.Context, method, path string, body any, target any) error {
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
	if c.tokenSource != nil {
		token, err := c.tokenSource.Token(ctx)
		if err != nil {
			return fmt.Errorf("product stock auth token: %w", err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	} else if req.Header.Get("Authorization") == "" {
		// No token source configured — product reservation routes require auth.
		return fmt.Errorf("product stock request failed: %w (no service-account token configured; set DUPLI1_ORDER_SERVICE_EMAIL/PASSWORD and DUPLI1_AUTH_URL)", ErrUnauthorized)
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
		msg := strings.TrimSpace(errBody.Error)
		if msg == "" {
			msg = resp.Status
		}
		if resp.StatusCode == http.StatusUnauthorized {
			if msg == "" || strings.EqualFold(msg, "unauthorized") {
				return fmt.Errorf("product stock request failed: %w", ErrUnauthorized)
			}
			return fmt.Errorf("product stock request failed: %w (%s)", ErrUnauthorized, msg)
		}
		return fmt.Errorf("product stock request failed: %s", msg)
	}

	if target != nil {
		return json.NewDecoder(resp.Body).Decode(target)
	}
	return nil
}
