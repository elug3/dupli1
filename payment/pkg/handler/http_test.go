package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/handler"
	"github.com/elug3/dupli1/payment/pkg/infra/checkout"
	"github.com/elug3/dupli1/payment/pkg/infra/memory"
	"github.com/elug3/dupli1/payment/pkg/ports"
	"github.com/elug3/dupli1/payment/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/golang-jwt/jwt/v5"
)

type stubOrderClient struct{}

func (s stubOrderClient) GetOrder(_ context.Context, _, _ string) (*ports.OrderSummary, error) {
	return &ports.OrderSummary{ID: "ord-1", CustomerID: "u-1", Status: "pending", TotalCents: 1000}, nil
}

func makeToken(t *testing.T, secret, userID string, perms []string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":         userID,
		"permissions": perms,
		"exp":         time.Now().Add(time.Hour).Unix(),
		"iat":         time.Now().Unix(),
		"type":        "access",
	})
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSettingsDoesNotRequireAuth(t *testing.T) {
	repo := memory.NewRepository()
	svc := service.New(repo, stubOrderClient{}, checkout.NewDevProvider("http://localhost:8080"), nil)
	h := handler.New(svc, authjwt.NewHMACValidator("test-secret"))
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
	h := handler.New(svc, authjwt.NewHMACValidator("test-secret")).WithDevSimulate(false)
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

	h := handler.New(svc, authjwt.NewHMACValidator("test-secret")).WithDevSimulate(true)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/payments/"+created.ID+"/simulate-success", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}

func TestRequireAuthFailsClosedWithoutValidator(t *testing.T) {
	repo := memory.NewRepository()
	svc := service.New(repo, stubOrderClient{}, checkout.NewDevProvider("http://localhost:8080"), nil)
	h := handler.New(svc, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewReader([]byte(`{"order_id":"ord-1"}`)))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCreatePayment_BypassRequiresPermission(t *testing.T) {
	const secret = "test-secret"
	repo := memory.NewRepository()
	pub := &recordingPublisher{}
	svc := service.New(repo, stubOrderClient{}, checkout.NewDevProvider("http://localhost:8080"), pub)
	h := handler.New(svc, authjwt.NewHMACValidator(secret))
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]string{
		"order_id": "ord-1",
		"method":   "bypass",
		"note":     "cash",
	})

	// Customer token — forbidden
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "u-1", nil))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("customer bypass status = %d, want 403; body: %s", rec.Code, rec.Body.String())
	}

	// Manager with payment.bypass — succeeded
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "mgr-1", []string{permissions.PaymentBypass}))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("manager bypass status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var payment domain.Payment
	if err := json.NewDecoder(rec.Body).Decode(&payment); err != nil {
		t.Fatal(err)
	}
	if payment.Status != domain.StatusSucceeded || payment.Method != domain.MethodBypass {
		t.Fatalf("payment = %+v", payment)
	}
	if payment.CreatedBy != "mgr-1" || payment.Note != "cash" {
		t.Fatalf("audit: created_by=%q note=%q", payment.CreatedBy, payment.Note)
	}
	if len(pub.events) != 1 {
		t.Fatalf("events = %d", len(pub.events))
	}
}

func TestCreatePayment_BitcoinNotImplemented(t *testing.T) {
	const secret = "test-secret"
	repo := memory.NewRepository()
	svc := service.New(repo, stubOrderClient{}, checkout.NewDevProvider("http://localhost:8080"), nil)
	h := handler.New(svc, authjwt.NewHMACValidator(secret))
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]string{"order_id": "ord-1", "method": "bitcoin"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "u-1", nil))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501; body: %s", rec.Code, rec.Body.String())
	}
}

type recordingPublisher struct {
	events []ports.PaymentSucceededEvent
}

func (p *recordingPublisher) Publish(_ context.Context, subject string, event any) error {
	if subject == ports.PaymentSucceededSubject {
		p.events = append(p.events, event.(ports.PaymentSucceededEvent))
	}
	return nil
}

type nanoOrderClient struct{}

func (s nanoOrderClient) GetOrder(_ context.Context, _, _ string) (*ports.OrderSummary, error) {
	return &ports.OrderSummary{
		ID: "ord-1", CustomerID: "u-1", Status: "pending", TotalCents: 1000,
		RecipientName: "홍길동", RecipientPhone: "01012345678",
	}, nil
}

func TestNanoReturn_SucceedsAndRedirects(t *testing.T) {
	repo := memory.NewRepository()
	pub := &recordingPublisher{}
	nano := checkout.NewNanoProvider(checkout.NanoConfig{
		Ver: "240000005", ShopCode: "240000005", LoginID: "shoptest", APIKey: "test-key",
		PublicBaseURL: "http://localhost:8080",
		SuccessURL:    "http://localhost:5173/checkout/confirmation",
		FailureURL:    "http://localhost:5173/checkout",
	})
	svc := service.New(repo, nanoOrderClient{}, nano, pub)
	created, err := svc.CreatePayment(context.Background(), service.CreatePaymentInput{
		OrderID: "ord-1", CustomerID: "u-1", BearerToken: "token",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	h := handler.New(svc, authjwt.NewHMACValidator("test-secret")).WithNano(nano)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	ts := "1725440123456"
	hash := checkout.NanoHash("240000005", "shoptest", "240000005", "1000", ts, "test-key")
	form := "resultCode=0000&shopcode=240000005&compOrderNo=" + created.ID +
		"&reqPayAmt=1000&tranNo=tn_1&timestamp=" + ts + "&hashValue=" + hash
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/nano/return", bytes.NewBufferString(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if loc == "" || !bytes.Contains([]byte(loc), []byte("order_id=ord-1")) {
		t.Fatalf("Location = %q", loc)
	}
	if len(pub.events) != 1 {
		t.Fatalf("events = %d", len(pub.events))
	}
}

func TestNanoReturn_RejectsForgedMinimalCallback(t *testing.T) {
	repo := memory.NewRepository()
	pub := &recordingPublisher{}
	nano := checkout.NewNanoProvider(checkout.NanoConfig{
		Ver: "240000005", ShopCode: "240000005", LoginID: "shoptest", APIKey: "test-key",
		PublicBaseURL: "http://localhost:8080",
		SuccessURL:    "http://localhost:5173/checkout/confirmation",
	})
	svc := service.New(repo, nanoOrderClient{}, nano, pub)
	created, err := svc.CreatePayment(context.Background(), service.CreatePaymentInput{
		OrderID: "ord-1", CustomerID: "u-1", BearerToken: "token",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	h := handler.New(svc, authjwt.NewHMACValidator("test-secret")).WithNano(nano)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	form := "resultCode=0000&compOrderNo=" + created.ID
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/nano/return", bytes.NewBufferString(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusSeeOther {
		t.Fatalf("forged callback must not redirect as success")
	}
	if len(pub.events) != 0 {
		t.Fatalf("events = %d, want 0", len(pub.events))
	}
}
