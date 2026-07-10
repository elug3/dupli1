package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elug3/dupli1/cart/pkg/authjwt"
	"github.com/elug3/dupli1/cart/pkg/handler"
	"github.com/elug3/dupli1/cart/pkg/infra/memory"
	"github.com/elug3/dupli1/cart/pkg/ports"
	"github.com/elug3/dupli1/cart/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "handler-test-secret"

type fakeProduct struct{}

func (f *fakeProduct) GetVariant(_ context.Context, sku string) (*ports.VariantInfo, error) {
	return &ports.VariantInfo{
		SKU:            sku,
		ProductID:      "BOT-001",
		Color:          "Black",
		UnitPriceCents: 125000,
		ImageURL:       "https://example.com/img.jpg",
	}, nil
}

func (f *fakeProduct) GetVariantBySkuID(_ context.Context, skuID string) (*ports.VariantInfo, error) {
	return &ports.VariantInfo{
		SkuID:          skuID,
		SKU:            skuID,
		ProductID:      "BOT-001",
		Color:          "Black",
		UnitPriceCents: 125000,
		ImageURL:       "https://example.com/img.jpg",
	}, nil
}

func makeToken(t *testing.T, userID string, perms []string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":         userID,
		"type":        "access",
		"permissions": perms,
		"exp":         time.Now().Add(time.Hour).Unix(),
		"iat":         time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("makeToken: %v", err)
	}
	return signed
}

func newTestHandler(t *testing.T) *handler.Handler {
	t.Helper()
	repo := memory.NewRepository()
	svc := service.New(repo, &fakeProduct{}, nil)
	validator := authjwt.NewHMACValidator(testSecret)
	return handler.New(svc, validator)
}

func newMux(h *handler.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func TestGetCartRequiresAuth(t *testing.T) {
	mux := newMux(newTestHandler(t))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestCartCRUD(t *testing.T) {
	mux := newMux(newTestHandler(t))
	userID := "user-1"
	token := makeToken(t, userID, nil)

	body, _ := json.Marshal(map[string]any{"sku": "BOT-001-BLK", "quantity": 2})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cart/items", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST items status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var cart struct {
		CustomerID    string `json:"customer_id"`
		SubtotalCents int64  `json:"subtotal_cents"`
		Items         []struct {
			SKU string `json:"sku"`
		} `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&cart); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cart.CustomerID != userID {
		t.Fatalf("customer_id = %q, want %q", cart.CustomerID, userID)
	}
	if cart.SubtotalCents != 250000 {
		t.Fatalf("subtotal = %d, want 250000", cart.SubtotalCents)
	}
	if len(cart.Items) != 1 || cart.Items[0].SKU != "BOT-001-BLK" {
		t.Fatalf("unexpected items: %+v", cart.Items)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET cart status = %d", rec.Code)
	}
}

func TestAdminCartForbiddenForCustomer(t *testing.T) {
	mux := newMux(newTestHandler(t))
	token := makeToken(t, "user-1", nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/carts/other-user", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestAdminCartAllowed(t *testing.T) {
	h := newTestHandler(t)
	mux := newMux(h)
	customerToken := makeToken(t, "customer-1", nil)
	adminToken := makeToken(t, "admin-1", permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin}))

	body, _ := json.Marshal(map[string]any{"sku": "BOT-001-BLK", "quantity": 1})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cart/items", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+customerToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("seed cart: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/carts/customer-1", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin get cart status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
