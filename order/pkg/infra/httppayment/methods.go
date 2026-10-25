package httppayment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/elug3/dupli1/order/pkg/ports"
)

// methodsTimeout bounds the kill switch lookup on the checkout path, which
// must not wait out the long cancel timeout when payment is slow.
const methodsTimeout = 3 * time.Second

// MethodDisabled asks payment's public GET /api/v1/payments/methods whether
// method's kill switch is engaged. A method payment does not list is not
// switchable and so never disabled.
func (c *Client) MethodDisabled(ctx context.Context, method string) (bool, string, error) {
	ctx, cancel := context.WithTimeout(ctx, methodsTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/payments/methods", nil)
	if err != nil {
		return false, "", fmt.Errorf("%w: %v", ports.ErrPaymentUnavailable, err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, "", fmt.Errorf("%w: %v", ports.ErrPaymentUnavailable, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("%w: %s", ports.ErrPaymentUnavailable, errorMessage(raw, resp.Status))
	}
	var body struct {
		Methods []struct {
			Method   string `json:"method"`
			Disabled bool   `json:"disabled"`
			Message  string `json:"message"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return false, "", fmt.Errorf("%w: decode methods: %v", ports.ErrPaymentUnavailable, err)
	}
	for _, m := range body.Methods {
		if m.Method == method {
			return m.Disabled, m.Message, nil
		}
	}
	return false, "", nil
}
