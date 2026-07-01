package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elug3/dupli1/product/pkg/authjwt"
	"github.com/elug3/dupli1/product/pkg/middleware"
	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "middleware-test-secret"

func makeAccessToken(t *testing.T, userID string, roles []string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":   userID,
		"roles": roles,
		"type":  "access",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("makeAccessToken: %v", err)
	}
	return signed
}

func TestRequireAuthRejectsMissingToken(t *testing.T) {
	validator := authjwt.NewHMACValidator(testSecret)
	called := false
	handler := middleware.RequireAuth(validator, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if called {
		t.Fatal("handler should not be called")
	}
}

func TestRequireAnyRoleAllowsProductManager(t *testing.T) {
	validator := authjwt.NewHMACValidator(testSecret)
	token := makeAccessToken(t, "mgr-1", []string{"product_manager"})
	called := false

	handler := middleware.RequireAuth(validator,
		middleware.RequireAnyRole(middleware.ProductManagerRoles...)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		})))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/products", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !called {
		t.Fatal("handler should be called")
	}
}

func TestRequireAnyRoleAllowsOwner(t *testing.T) {
	validator := authjwt.NewHMACValidator(testSecret)
	token := makeAccessToken(t, "owner-1", []string{"owner", "product_manager"})
	called := false

	handler := middleware.RequireAuth(validator,
		middleware.RequireAnyRole(middleware.ProductManagerRoles...)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		})))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/products", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !called {
		t.Fatal("handler should be called")
	}
}

func TestRequireAnyRoleRejectsCustomer(t *testing.T) {
	validator := authjwt.NewHMACValidator(testSecret)
	token := makeAccessToken(t, "cust-1", []string{"customer"})
	called := false

	handler := middleware.RequireAuth(validator,
		middleware.RequireAnyRole(middleware.ProductManagerRoles...)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		})))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/products", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
	if called {
		t.Fatal("handler should not be called")
	}
}
