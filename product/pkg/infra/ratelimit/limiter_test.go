package ratelimit_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elug3/dupli1/product/pkg/infra/ratelimit"
)

func TestMemoryCounterEnforcesTheBudgetThenResets(t *testing.T) {
	limiter := ratelimit.New(ratelimit.NewMemoryCounter(), 3, 50*time.Millisecond)
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		if !limiter.Allow(ctx, "ip:1.2.3.4") {
			t.Fatalf("request %d should be allowed within a budget of 3", i)
		}
	}
	if limiter.Allow(ctx, "ip:1.2.3.4") {
		t.Fatal("the fourth request should be refused")
	}
	// A different key has its own budget.
	if !limiter.Allow(ctx, "ip:5.6.7.8") {
		t.Fatal("a different caller should not inherit the first one's budget")
	}

	time.Sleep(60 * time.Millisecond)
	if !limiter.Allow(ctx, "ip:1.2.3.4") {
		t.Fatal("the window should have reset")
	}
}

// A limiter outage must not stop customers checking out. The ledger, not this,
// is what limits actual use.
type brokenCounter struct{}

func (brokenCounter) Incr(context.Context, string, time.Duration) (int, error) {
	return 0, errors.New("backend down")
}

func TestLimiterFailsOpenWhenTheBackendIsDown(t *testing.T) {
	limiter := ratelimit.New(brokenCounter{}, 1, time.Minute)
	for i := 0; i < 5; i++ {
		if !limiter.Allow(context.Background(), "ip:1.2.3.4") {
			t.Fatal("a broken backend must not block requests")
		}
	}
}

func TestNilLimiterAllows(t *testing.T) {
	var limiter *ratelimit.Limiter
	if !limiter.Allow(context.Background(), "ip:1.2.3.4") {
		t.Fatal("an unconfigured limiter should allow everything")
	}
}

func TestMiddlewareRefusesOverBudgetWith429(t *testing.T) {
	limiter := ratelimit.New(ratelimit.NewMemoryCounter(), 1, time.Minute)
	handler := limiter.Middleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	call := func() int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/products/promotions/redeem", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.9")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w.Code
	}
	if got := call(); got != http.StatusOK {
		t.Fatalf("first call = %d, want 200", got)
	}
	if got := call(); got != http.StatusTooManyRequests {
		t.Fatalf("second call = %d, want 429", got)
	}
}

// Both budgets are counted: an IP budget alone is defeated by a botnet, and a
// customer budget alone by staying anonymous.
func TestMiddlewareAlsoBudgetsPerCustomer(t *testing.T) {
	limiter := ratelimit.New(ratelimit.NewMemoryCounter(), 1, time.Minute)
	customerOf := func(r *http.Request) string { return r.Header.Get("X-Test-Customer") }
	handler := limiter.Middleware(customerOf)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	call := func(ip, customer string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/products/promotions/redeem", nil)
		req.Header.Set("X-Forwarded-For", ip)
		req.Header.Set("X-Test-Customer", customer)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w.Code
	}
	if got := call("203.0.113.1", "cust-1"); got != http.StatusOK {
		t.Fatalf("first call = %d, want 200", got)
	}
	// Same customer from a fresh IP is still over their own budget.
	if got := call("203.0.113.2", "cust-1"); got != http.StatusTooManyRequests {
		t.Fatalf("same customer on a new IP = %d, want 429", got)
	}
}

// Every request reaches this service through nginx, so RemoteAddr alone would
// bucket the whole world onto the proxy.
func TestClientIPPrefersTheForwardedCaller(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:34567"
	req.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.5")
	if got := ratelimit.ClientIP(req); got != "203.0.113.7" {
		t.Fatalf("ClientIP = %q, want the left-most forwarded entry", got)
	}
}

func TestClientIPFallsBackToRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.4:9999"
	if got := ratelimit.ClientIP(req); got != "198.51.100.4" {
		t.Fatalf("ClientIP = %q, want 198.51.100.4", got)
	}
}
