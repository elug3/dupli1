// Package httppromotion talks to product, which owns promotional code
// definitions and the usage ledger.
//
// It replaces a redeem-only client that asked product whether a code existed
// and then computed the discount here. Pricing a promotion needs the cart —
// minimum spend, eligible lines, caps — so product now does the arithmetic and
// this client carries the cart there.
package httppromotion

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
	baseURL    string
	httpClient *http.Client
	// tokenSource supplies the Bearer token for the ledger endpoints, which
	// are service-to-service and need promotion.redeem. Evaluate is public and
	// needs none. This is the same source the stock and payment clients use,
	// so order's service account is configured in one place.
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

type evaluateRequest struct {
	Code           string                `json:"code"`
	OrderID        string                `json:"order_id,omitempty"`
	CustomerID     string                `json:"customer_id"`
	ShippingFeeWon int64                 `json:"shipping_fee_won"`
	Lines          []ports.PromotionLine `json:"lines"`
}

func (c *Client) Evaluate(ctx context.Context, code string, promoCtx ports.PromotionContext) (*ports.PromotionEvaluation, error) {
	return c.evaluateAt(ctx, "/api/v1/products/promotions/evaluate", evaluateRequest{
		Code:           code,
		CustomerID:     promoCtx.CustomerID,
		ShippingFeeWon: promoCtx.ShippingFeeWon,
		Lines:          promoCtx.Lines,
	}, false)
}

func (c *Client) Reserve(ctx context.Context, code, orderID string, promoCtx ports.PromotionContext) (*ports.PromotionEvaluation, error) {
	return c.evaluateAt(ctx, "/api/v1/products/promotions/reserve", evaluateRequest{
		Code:           code,
		OrderID:        orderID,
		CustomerID:     promoCtx.CustomerID,
		ShippingFeeWon: promoCtx.ShippingFeeWon,
		Lines:          promoCtx.Lines,
	}, true)
}

func (c *Client) evaluateAt(ctx context.Context, path string, req evaluateRequest, authed bool) (*ports.PromotionEvaluation, error) {
	resp, err := c.post(ctx, path, req, authed)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &ports.PromotionEvaluation{Code: req.Code, Reason: "invalid_code"}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("promotion %s failed: %s", path, describeError(resp))
	}

	// Reserve wraps the verdict alongside the ledger row; evaluate returns it
	// bare. Decoding into a struct that carries both keeps one code path.
	var body struct {
		ports.PromotionEvaluation
		Result *ports.PromotionEvaluation `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := body.PromotionEvaluation
	if body.Result != nil {
		out = *body.Result
	}
	out.Code = req.Code
	return &out, nil
}

func (c *Client) Consume(ctx context.Context, orderID string) error {
	return c.orderTransition(ctx, "/api/v1/products/promotions/consume", orderID)
}

func (c *Client) Release(ctx context.Context, orderID string) error {
	return c.orderTransition(ctx, "/api/v1/products/promotions/release", orderID)
}

func (c *Client) orderTransition(ctx context.Context, path, orderID string) error {
	resp, err := c.post(ctx, path, map[string]string{"order_id": orderID}, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("promotion %s failed: %s", path, describeError(resp))
	}
	return nil
}

func (c *Client) post(ctx context.Context, path string, payload any, authed bool) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if authed && c.tokenSource != nil {
		token, err := c.tokenSource.Token(ctx)
		if err != nil {
			return nil, fmt.Errorf("promotion auth: %w", err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	return c.httpClient.Do(req)
}

func describeError(resp *http.Response) string {
	var errBody struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&errBody)
	if errBody.Error == "" {
		return resp.Status
	}
	return errBody.Error
}
