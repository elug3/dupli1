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

// The review routes live beside /products/{id}/... patterns; only the full
// mux proves they register without a conflict and carry the right guards.
func TestReviewRoutesThroughBootstrap(t *testing.T) {
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		t.Skip("POSTGRES_URL not set")
	}
	const secret = "review-routes-secret"
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
	moderator := sign(jwt.MapClaims{"sub": "m1", "account_type": "manager", "permissions": []string{permissions.ProductReviewModerate}})

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
	cases := []struct {
		method, path, token, body string
		want                      int
	}{
		{"GET", "/api/v1/products/NO-SUCH/reviews", "", "", http.StatusNotFound},
		{"GET", "/api/v1/products/NO-SUCH/reviews/photos", "", "", http.StatusNotFound},
		{"POST", "/api/v1/products/NO-SUCH/reviews", "", "{}", http.StatusUnauthorized},
		{"GET", "/api/v1/products/reviews/me", "", "", http.StatusUnauthorized},
		{"GET", "/api/v1/products/reviews/me", shopper, "", http.StatusOK},
		{"GET", "/api/v1/products/reviews", shopper, "", http.StatusForbidden},
		{"GET", "/api/v1/products/reviews", moderator, "", http.StatusOK},
		{"POST", "/api/v1/products/reviews/by-id/R1/hide", shopper, `{"reason":"x"}`, http.StatusForbidden},
		{"POST", "/api/v1/products/reviews/by-id/R1/hide", moderator, `{"reason":"x"}`, http.StatusNotFound},
		{"PATCH", "/api/v1/products/reviews/by-id/R1", shopper, `{"rating":5}`, http.StatusNotFound},
		// No gateway configured: new reviews are refused, not half-checked.
		{"POST", "/api/v1/products/NO-SUCH/reviews", shopper, `{"orderId":"O","skuId":"S","rating":5,"body":"0123456789"}`, http.StatusServiceUnavailable},
		// No object storage configured.
		{"POST", "/api/v1/products/reviews/photos", shopper, "", http.StatusServiceUnavailable},
	}
	for _, c := range cases {
		if got := do(c.method, c.path, c.token, c.body); got != c.want {
			t.Errorf("%s %s: status %d, want %d", c.method, c.path, got, c.want)
		}
	}
}
