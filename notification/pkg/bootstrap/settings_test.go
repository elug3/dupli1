package bootstrap_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elug3/dupli1/notification/pkg/bootstrap"
)

func TestSettingsEndpoint(t *testing.T) {
	app, err := bootstrap.Bootstrap(bootstrap.Config{
		Addr:         ":0",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	for _, path := range []string{"/settings", "/api/v1/notification/settings"} {
		rec := httptest.NewRecorder()
		app.HTTP.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, rec.Code)
		}
		var body map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("%s decode: %v", path, err)
		}
		if body["service"] != "notification" {
			t.Fatalf("%s service = %v, want notification", path, body["service"])
		}
	}
}
