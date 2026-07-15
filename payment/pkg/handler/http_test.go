package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/payment/pkg/authjwt"
	"github.com/elug3/dupli1/payment/pkg/handler"
	"github.com/elug3/dupli1/payment/pkg/infra/checkout"
	"github.com/elug3/dupli1/payment/pkg/infra/memory"
	"github.com/elug3/dupli1/payment/pkg/ports"
	"github.com/elug3/dupli1/payment/pkg/service"
)

type stubOrderClient struct{}

func (s stubOrderClient) GetOrder(_ context.Context, _, _ string) (*ports.OrderSummary, error) {
	return &ports.OrderSummary{ID: "ord-1", CustomerID: "u-1", Status: "pending", TotalCents: 1000}, nil
}

func TestSettingsDoesNotRequireAuth(t *testing.T) {
	repo := memory.NewRepository()
	svc := service.New(repo, stubOrderClient{}, checkout.NewDevProvider("http://localhost:8080"), nil)
	h := handler.New(svc, authjwt.NewHMACValidator("test-secret"), "")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	for _, path := range []string{"/settings", "/api/v1/payments/settings"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, rec.Code)
		}
		var body map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("%s decode: %v", path, err)
		}
		if body["service"] != "payment" {
			t.Fatalf("%s service = %v, want payment", path, body["service"])
		}
	}
}
