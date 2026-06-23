package product

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const productTestSecret = "product-test-secret"

func makeProductToken(t *testing.T, userID string, roles []string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":   userID,
		"roles": roles,
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(productTestSecret))
	if err != nil {
		t.Fatalf("makeProductToken: %v", err)
	}
	return signed
}

func newTestProductHandler(t *testing.T) *ProductSearchHandler {
	t.Helper()
	svc := NewProductSearchService(fakeProductStore{})
	return NewProductSearchHandler(svc, productTestSecret)
}

func newProductMux(h *ProductSearchHandler) *http.ServeMux {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

// Protected endpoints that require auth.
var protectedProductPaths = []string{
	"/api/v1/products/all",
	"/api/v1/products/categories",
	"/api/v1/products/filters?category=shoes",
	"/api/v1/products/search?category=shoes",
	"/api/v1/products/shoes",
}

func TestProductEndpoint_NoToken_Returns401(t *testing.T) {
	h := newTestProductHandler(t)
	mux := newProductMux(h)

	for _, path := range protectedProductPaths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("%s: status = %d, want 401", path, w.Code)
			}
		})
	}
}

func TestProductEndpoint_InvalidToken_Returns401(t *testing.T) {
	h := newTestProductHandler(t)
	mux := newProductMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/products/all", nil)
	req.Header.Set("Authorization", "Bearer not.a.real.jwt")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestProductEndpoint_CustomerTokenPasses(t *testing.T) {
	h := newTestProductHandler(t)
	mux := newProductMux(h)
	token := makeProductToken(t, "u-1", []string{"customer"})

	for _, path := range protectedProductPaths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("%s: status = %d, want 200", path, w.Code)
			}
		})
	}
}

func TestProductEndpoint_OrderManagerTokenPasses(t *testing.T) {
	h := newTestProductHandler(t)
	mux := newProductMux(h)
	token := makeProductToken(t, "mgr-1", []string{"order_manager"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/products/all", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestHealthEndpoint_NoAuthRequired(t *testing.T) {
	h := newTestProductHandler(t)
	mux := newProductMux(h)

	for _, path := range []string{"/health", "/api/v1/products/health"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("%s: status = %d, want 200", path, w.Code)
			}
		})
	}
}
