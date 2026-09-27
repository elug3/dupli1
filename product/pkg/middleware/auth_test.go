package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/product/pkg/middleware"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "middleware-test-secret"

func makeAccessToken(t *testing.T, userID string, perms []string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":         userID,
		"permissions": perms,
		"type":        "access",
		"exp":         time.Now().Add(time.Hour).Unix(),
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

func TestRequireAnyPermissionAllowsProductManager(t *testing.T) {
	validator := authjwt.NewHMACValidator(testSecret)
	token := makeAccessToken(t, "mgr-1", permissions.ExpandLegacyRoles([]string{permissions.RoleProductManager}))
	called := false

	handler := middleware.RequireAuth(validator,
		middleware.RequireAnyPermission(permissions.ProductCreate)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestRequireAnyPermissionAllowsOwner(t *testing.T) {
	validator := authjwt.NewHMACValidator(testSecret)
	token := makeAccessToken(t, "owner-1", []string{permissions.All})
	called := false

	handler := middleware.RequireAuth(validator,
		middleware.RequireAnyPermission(permissions.ProductCreate)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestRequireAnyPermissionRejectsCustomer(t *testing.T) {
	validator := authjwt.NewHMACValidator(testSecret)
	token := makeAccessToken(t, "cust-1", nil)
	called := false

	handler := middleware.RequireAuth(validator,
		middleware.RequireAnyPermission(permissions.ProductCreate)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestRequireServiceAllowsOnlyNamedService(t *testing.T) {
	validator := authjwt.NewHMACValidator(testSecret)
	sign := func(claims jwt.MapClaims) string {
		claims["type"] = "access"
		claims["exp"] = time.Now().Add(time.Hour).Unix()
		signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		return signed
	}
	cases := []struct {
		name   string
		claims jwt.MapClaims
		want   int
	}{
		{"order service", jwt.MapClaims{"sub": "s1", "account_type": "service", "service_name": "dupli1-order"}, http.StatusOK},
		{"web service", jwt.MapClaims{"sub": "s2", "account_type": "service", "service_name": "dupli1-web"}, http.StatusForbidden},
		{"owner", jwt.MapClaims{"sub": "o1", "account_type": "manager", "permissions": []string{permissions.All}}, http.StatusForbidden},
		{"customer", jwt.MapClaims{"sub": "c1", "account_type": "customer", "service_name": "dupli1-order"}, http.StatusForbidden},
	}
	for _, tc := range cases {
		h := middleware.RequireAuth(validator, middleware.RequireService("dupli1-order")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Authorization", "Bearer "+sign(tc.claims))
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, rr.Code, tc.want)
		}
	}
}
