// Package httppoints talks to payment's internal points hold routes
// (POST /api/v1/payments/points/reserve|consume|release). They answer only the
// dupli1-order service account on the gateway's internal listener, so the base
// URL is order's DUPLI1_GATEWAY_URL and the token is order's own.
package httppoints

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/elug3/dupli1/order/pkg/infra/httpauth"
	"github.com/elug3/dupli1/order/pkg/ports"
)

type Client struct {
	baseURL     string
	httpClient  *http.Client
	tokenSource httpauth.TokenSource
}

func NewClient(baseURL string, httpClient *http.Client, tokenSource httpauth.TokenSource) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient, tokenSource: tokenSource}
}

func (c *Client) Reserve(ctx context.Context, orderID, customerID string, points int64) error {
	return c.post(ctx, "reserve", map[string]any{"order_id": orderID, "customer_id": customerID, "points": points}, false)
}

func (c *Client) Consume(ctx context.Context, orderID string) error {
	return c.post(ctx, "consume", map[string]any{"order_id": orderID}, false)
}

func (c *Client) Release(ctx context.Context, orderID string) error {
	return c.post(ctx, "release", map[string]any{"order_id": orderID}, true)
}

// post calls one hold route. A refusal that the shopper caused (too few
// points, a bad amount) maps to a ports error the handler can show; anything
// else is ErrPointsUnavailable. missingOK treats "no hold" as done.
func (c *Client) post(ctx context.Context, action string, payload any, missingOK bool) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/payments/points/"+action, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.tokenSource != nil {
		token, err := c.tokenSource.Token(ctx)
		if err != nil {
			return fmt.Errorf("%w: points auth: %v", ports.ErrPointsUnavailable, err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: points %s: %v", ports.ErrPointsUnavailable, action, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	var e struct {
		Error  string `json:"error"`
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&e)
	switch {
	case e.Reason == "insufficient_points":
		return ports.ErrInsufficientPoints
	case e.Reason == "invalid_points":
		return ports.ErrInvalidPoints
	case missingOK && e.Reason == "points_hold_not_found":
		return nil
	}
	msg := e.Reason
	if msg == "" {
		msg = e.Error
	}
	if msg == "" {
		msg = resp.Status
	}
	return fmt.Errorf("%w: points %s: %s", ports.ErrPointsUnavailable, action, msg)
}
