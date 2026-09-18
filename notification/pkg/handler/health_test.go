package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/elug3/dupli1/notification/pkg/handler"
)

func getHealth(t *testing.T, h *handler.Handler) (int, map[string]any) {
	t.Helper()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return rec.Code, body
}

// With no dependencies wired, /health stays the plain liveness answer.
func TestHealthWithoutProbes(t *testing.T) {
	code, body := getHealth(t, handler.New(handler.Options{}))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %v, want ok", body["status"])
	}
	if _, ok := body["dependencies"]; ok {
		t.Fatalf("expected no dependencies key, got %v", body)
	}
}

func TestHealthReportsHealthyDependencies(t *testing.T) {
	h := handler.New(handler.Options{HealthProbes: map[string]handler.HealthProbe{
		"postgres": func(context.Context) error { return nil },
		"nats":     func(context.Context) error { return nil },
	}})

	code, body := getHealth(t, h)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %v, want ok", body["status"])
	}
	deps, _ := body["dependencies"].(map[string]any)
	for _, name := range []string{"postgres", "nats"} {
		dep, _ := deps[name].(map[string]any)
		if dep["ok"] != true {
			t.Fatalf("%s = %v, want ok:true", name, dep)
		}
	}
}

// A failing dependency shows in the body, but the status code stays 200:
// nothing probes this endpoint, and a 503 would only set up a restart loop for
// whoever wires one to it later. The cause is logged, never returned — /health
// is unauthenticated.
func TestHealthReportsDegradedWithoutFailingTheRequest(t *testing.T) {
	h := handler.New(handler.Options{HealthProbes: map[string]handler.HealthProbe{
		"postgres": func(context.Context) error {
			return errors.New("dial tcp db.internal:5432: connection refused")
		},
		"nats": func(context.Context) error { return nil },
	}})

	code, body := getHealth(t, h)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 even when degraded", code)
	}
	if body["status"] != "degraded" {
		t.Fatalf("status = %v, want degraded", body["status"])
	}
	deps, _ := body["dependencies"].(map[string]any)
	pg, _ := deps["postgres"].(map[string]any)
	if pg["ok"] != false {
		t.Fatalf("postgres = %v, want ok:false", pg)
	}
	if _, leaked := pg["error"]; leaked {
		t.Fatalf("probe error must not reach an unauthenticated response: %v", pg)
	}
	if nats, _ := deps["nats"].(map[string]any); nats["ok"] != true {
		t.Fatalf("nats = %v, want ok:true", nats)
	}
}

// Without caching, an unauthenticated endpoint would ping the database as fast
// as anyone can ask for it.
func TestHealthCachesProbeResults(t *testing.T) {
	var calls int32
	h := handler.New(handler.Options{HealthProbes: map[string]handler.HealthProbe{
		"postgres": func(context.Context) error {
			atomic.AddInt32(&calls, 1)
			return nil
		},
	}})

	for range 5 {
		if code, _ := getHealth(t, h); code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("probe ran %d times, want 1 within the cache window", got)
	}
}
