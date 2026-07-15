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

func TestSimulateSuccess_DisabledWhenNotDev(t *testing.T) {
	repo := memory.NewRepository()
	svc := service.New(repo, stubOrderClient{}, checkout.NewDevProvider("http://localhost:8080"), nil)
	h := handler.New(svc, authjwt.NewHMACValidator("test-secret"), "").WithDevSimulate(false)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/payments/pay-1/simulate-success", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestSimulateSuccess_EnabledInDev(t *testing.T) {
	repo := memory.NewRepository()
	svc := service.New(repo, stubOrderClient{}, checkout.NewDevProvider("http://localhost:8080"), nil)
	created, err := svc.CreatePayment(context.Background(), service.CreatePaymentInput{
		OrderID: "ord-1", CustomerID: "u-1", BearerToken: "token",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	h := handler.New(svc, authjwt.NewHMACValidator("test-secret"), "").WithDevSimulate(true)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/payments/"+created.ID+"/simulate-success", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}
