package httppayment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/elug3/dupli1/order/pkg/infra/httpauth"
	"github.com/elug3/dupli1/order/pkg/ports"
)

const defaultTimeout = 25 * time.Second

// Client calls payment cancel over the internal gateway.
type Client struct {
	baseURL     string
	httpClient  *http.Client
	tokenSource httpauth.TokenSource
}

func NewClient(baseURL string, httpClient *http.Client, tokenSource httpauth.TokenSource) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		httpClient:  httpClient,
		tokenSource: tokenSource,
	}
}

// NewClientWithBearer builds a client with a fixed bearer token (tests).
func NewClientWithBearer(baseURL string, httpClient *http.Client, bearerToken string) *Client {
	var src httpauth.TokenSource
	if bearerToken != "" {
		src = httpauth.StaticToken(bearerToken)
	}
	return NewClient(baseURL, httpClient, src)
}

func (c *Client) CancelPayment(ctx context.Context, paymentID, idempotencyKey string) error {
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return fmt.Errorf("%w: missing payment id", ports.ErrPaymentUnavailable)
	}
	err := c.doCancel(ctx, paymentID, idempotencyKey)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ports.ErrPaymentUnauthorized) {
		return err
	}
	if inv, ok := c.tokenSource.(interface{ Invalidate() }); ok {
		inv.Invalidate()
	}
	return c.doCancel(ctx, paymentID, idempotencyKey)
}

func (c *Client) doCancel(ctx context.Context, paymentID, idempotencyKey string) error {
	body, err := json.Marshal(map[string]string{"reason": "order canceled"})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/payments/"+paymentID+"/cancel", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: %v", ports.ErrPaymentUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(idempotencyKey); key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	token, err := c.bearer(ctx)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ports.ErrPaymentUnavailable, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	msg := errorMessage(raw, resp.Status)
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w (%s)", ports.ErrPaymentUnauthorized, msg)
	case http.StatusForbidden:
		return fmt.Errorf("%w: %s", ports.ErrPaymentForbidden, msg)
	case http.StatusConflict:
		return c.conflictMeansAlreadyRefunded(ctx, paymentID, token, msg)
	case http.StatusBadGateway, http.StatusNotImplemented:
		return fmt.Errorf("%w: %s", ports.ErrPaymentRefundRejected, msg)
	default:
		return fmt.Errorf("%w: %s", ports.ErrPaymentUnavailable, msg)
	}
}

func (c *Client) conflictMeansAlreadyRefunded(ctx context.Context, paymentID, token, conflictMsg string) error {
	status, err := c.paymentStatus(ctx, paymentID, token)
	if err != nil {
		return fmt.Errorf("%w: %s", ports.ErrPaymentRefundRejected, conflictMsg)
	}
	if status == "canceled" {
		return nil
	}
	return fmt.Errorf("%w: %s", ports.ErrPaymentRefundRejected, conflictMsg)
}

func (c *Client) paymentStatus(ctx context.Context, paymentID, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/payments/"+paymentID, nil)
	if err != nil {
		return "", err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("payment get http %d", resp.StatusCode)
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&body); err != nil {
		return "", err
	}
	return strings.TrimSpace(body.Status), nil
}

func (c *Client) bearer(ctx context.Context) (string, error) {
	if token := ports.PaymentBearer(ctx); token != "" {
		return token, nil
	}
	if c.tokenSource == nil {
		return "", fmt.Errorf("%w: no payment auth token (set DUPLI1_ORDER_SERVICE_EMAIL/PASSWORD)", ports.ErrPaymentUnauthorized)
	}
	token, err := c.tokenSource.Token(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ports.ErrPaymentUnauthorized, err)
	}
	return token, nil
}

func errorMessage(raw []byte, fallback string) string {
	var body struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &body) == nil && strings.TrimSpace(body.Error) != "" {
		return strings.TrimSpace(body.Error)
	}
	msg := strings.TrimSpace(string(raw))
	if msg != "" {
		return msg
	}
	return fallback
}
