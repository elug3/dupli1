package httpcoupon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/elug3/dupli1/order/pkg/ports"
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

func (c *Client) Redeem(ctx context.Context, code string) (*ports.Coupon, error) {
	var response struct {
		Code     string  `json:"code"`
		Discount float64 `json:"discount"`
		Active   bool    `json:"active"`
	}

	payload := map[string]string{"code": code}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/products/coupons/redeem", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ports.ErrCouponInvalid
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		if errBody.Error == "" {
			errBody.Error = resp.Status
		}
		return nil, fmt.Errorf("coupon request failed: %s", errBody.Error)
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	if !response.Active || response.Discount <= 0 {
		return nil, ports.ErrCouponInvalid
	}

	return &ports.Coupon{
		Code:             response.Code,
		DiscountFraction: response.Discount,
	}, nil
}
