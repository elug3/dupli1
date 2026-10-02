package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

const visitBrowserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"

// Visit throttling is wired in Bootstrap, not the handler package tests alone.
func TestVisitBeaconRateLimitPerIP(t *testing.T) {
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		t.Skip("POSTGRES_URL not set")
	}
	app, err := Bootstrap(context.Background(), Config{DatabaseConnString: dsn, JWTSecret: "visit-rate-limit-secret"})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	const budget = 120
	ip := "203.0.113.55"
	post := func() int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/products/visits", nil)
		req.Header.Set("User-Agent", visitBrowserUA)
		req.Header.Set("X-Forwarded-For", ip)
		w := httptest.NewRecorder()
		app.Handler.ServeHTTP(w, req)
		return w.Code
	}
	for i := 0; i < budget; i++ {
		if got := post(); got != http.StatusNoContent {
			t.Fatalf("visit %d: status=%d, want 204", i+1, got)
		}
	}
	if got := post(); got != http.StatusTooManyRequests {
		t.Fatalf("visit %d: status=%d, want 429", budget+1, got)
	}
}
