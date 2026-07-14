package settings_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/settings"
)

func TestHostFromURL(t *testing.T) {
	t.Parallel()
	host, ok := settings.HostFromURL("postgres://dupli1:secret@db.example:5432/orders?sslmode=disable")
	if !ok || host != "db.example" {
		t.Fatalf("got host=%q ok=%v", host, ok)
	}
	host, ok = settings.HostFromURL("http://dupli1-product:8080")
	if !ok || host != "dupli1-product" {
		t.Fatalf("got host=%q ok=%v", host, ok)
	}
	if _, ok := settings.HostFromURL(""); ok {
		t.Fatal("empty URL should not be configured")
	}
}

func TestConsumerAuth(t *testing.T) {
	t.Parallel()
	a := settings.ConsumerAuth("http://auth/.well-known/jwks.json", "dev-secret")
	if a.Mode != "jwks" || !a.JWKSConfigured || !a.JWTSecretFallback {
		t.Fatalf("unexpected auth: %+v", a)
	}
	a = settings.ConsumerAuth("", "")
	if a.Mode != "none" {
		t.Fatalf("expected none, got %s", a.Mode)
	}
}

func TestHandler(t *testing.T) {
	t.Parallel()
	resp := settings.NewResponse("order")
	resp.Storage = "memory"
	mux := http.NewServeMux()
	mux.HandleFunc("/settings", settings.Handler(resp))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/settings", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var got settings.Response
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Service != "order" || got.APIVersion != "v1" || got.Storage != "memory" {
		t.Fatalf("unexpected body: %+v", got)
	}

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/settings", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestTimeoutSeconds(t *testing.T) {
	t.Parallel()
	if settings.TimeoutSeconds(5*time.Second) != 5 {
		t.Fatal("expected 5")
	}
}
