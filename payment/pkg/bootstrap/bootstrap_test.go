package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elug3/dupli1/payment/pkg/infra/checkout"
	"github.com/elug3/dupli1/payment/pkg/service"
)

func TestBootstrap_WiresUnavailableCheckoutWithoutNanoOrDevSimulate(t *testing.T) {
	orderSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/orders/ord_1" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "ord_1",
			"customer_id":  "cust_1",
			"status":       "pending",
			"total_cents":  1000,
			"recipient_name":  "Kim",
			"recipient_phone": "01012345678",
		})
	}))
	defer orderSrv.Close()

	app, err := Bootstrap(Config{
		OrderURL:  orderSrv.URL,
		JWTSecret: "dev-secret",
	})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	defer app.Close()

	_, err = app.Service.CreatePayment(context.Background(), service.CreatePaymentInput{
		OrderID:    "ord_1",
		CustomerID: "cust_1",
		Method:     "credit_card",
	})
	if err == nil {
		t.Fatal("expected credit_card to fail when no PG is configured")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("error = %v, want unavailable provider message", err)
	}
}

func TestBootstrap_PrefersNanoOverDevSimulate(t *testing.T) {
	cfg := Config{
		OrderURL:         "http://order",
		JWTSecret:        "dev-secret",
		AllowDevSimulate: true,
		Nano: checkout.NanoConfig{
			ShopCode: "240000005",
			LoginID:  "shoptest",
			APIKey:   "test-key",
		},
	}
	resp := BuildSettings(cfg)
	if resp.Features["dev_simulate_success"] {
		t.Fatal("nano configured: dev_simulate_success must be false")
	}
	if got := resp.Limits["checkout_provider"]; got != "nano" {
		t.Fatalf("checkout_provider = %v, want nano", got)
	}
}
