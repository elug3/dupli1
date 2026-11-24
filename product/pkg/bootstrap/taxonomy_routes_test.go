package bootstrap

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/golang-jwt/jwt/v5"
)

// Reading the taxonomy stays public (storefront filters); changing it needs
// product.master.write, and the writes reach the store through the real mux.
func TestTaxonomyRoutesThroughBootstrap(t *testing.T) {
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		t.Skip("POSTGRES_URL not set")
	}
	const secret = "taxonomy-routes-secret"
	app, err := Bootstrap(context.Background(), Config{DatabaseConnString: dsn, JWTSecret: secret})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	sign := func(claims jwt.MapClaims) string {
		claims["type"] = "access"
		claims["exp"] = time.Now().Add(time.Hour).Unix()
		s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	shopper := sign(jwt.MapClaims{"sub": "c1", "account_type": "customer"})
	reader := sign(jwt.MapClaims{"sub": "m1", "account_type": "manager", "permissions": []string{permissions.ProductMasterRead}})
	editor := sign(jwt.MapClaims{"sub": "m2", "account_type": "manager", "permissions": []string{permissions.ProductMasterWrite}})

	do := func(method, path, token, body string) int {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rr := httptest.NewRecorder()
		app.Handler.ServeHTTP(rr, req)
		return rr.Code
	}
	const code = "bootstrap-route-test"
	t.Cleanup(func() { do("DELETE", "/api/v1/products/catalog/targets/"+code, editor, "") })
	cases := []struct {
		method, path, token, body string
		want                      int
	}{
		{"GET", "/api/v1/products/catalog/targets", "", "", http.StatusOK},
		{"GET", "/api/v1/products/catalog/subcategories?category=clothing", "", "", http.StatusOK},
		{"POST", "/api/v1/products/catalog/targets", "", `{"code":"` + code + `","name":"T"}`, http.StatusUnauthorized},
		{"POST", "/api/v1/products/catalog/targets", shopper, `{"code":"` + code + `","name":"T"}`, http.StatusForbidden},
		{"POST", "/api/v1/products/catalog/targets", reader, `{"code":"` + code + `","name":"T"}`, http.StatusForbidden},
		{"POST", "/api/v1/products/catalog/targets", editor, `{"code":"` + code + `","name":"T"}`, http.StatusCreated},
		{"PATCH", "/api/v1/products/catalog/targets/" + code, reader, `{"name":"U"}`, http.StatusForbidden},
		{"PATCH", "/api/v1/products/catalog/targets/" + code, editor, `{"name":"U"}`, http.StatusOK},
		{"DELETE", "/api/v1/products/catalog/targets/" + code, shopper, "", http.StatusForbidden},
		{"DELETE", "/api/v1/products/catalog/targets/" + code, editor, "", http.StatusNoContent},
		{"POST", "/api/v1/products/catalog/bag-styles", editor, `{"code":"bad code","name":"X"}`, http.StatusBadRequest},
		{"DELETE", "/api/v1/products/catalog/subcategories/no-such-term", editor, "", http.StatusNotFound},
	}
	for _, c := range cases {
		if got := do(c.method, c.path, c.token, c.body); got != c.want {
			t.Errorf("%s %s: status %d, want %d", c.method, c.path, got, c.want)
		}
	}
}
