package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/settings"
	tg "github.com/elug3/dupli1/shared/pkg/telegram"
	"github.com/elug3/dupli1/support/pkg/handler"
)

type recorder struct {
	mu      sync.Mutex
	updates []tg.Update
	done    chan struct{}
}

func newRecorder() *recorder { return &recorder{done: make(chan struct{}, 8)} }

func (r *recorder) Handle(_ context.Context, update tg.Update) error {
	r.mu.Lock()
	r.updates = append(r.updates, update)
	r.mu.Unlock()
	r.done <- struct{}{}
	return nil
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.updates)
}

func newServer(t *testing.T, secret string, updates handler.UpdateHandler) *http.ServeMux {
	t.Helper()
	h := handler.New(handler.Options{
		Updates:       updates,
		WebhookSecret: secret,
		Settings:      settings.NewResponse("support"),
	})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func post(t *testing.T, mux *http.ServeMux, secret, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/support/telegram/webhook", strings.NewReader(body))
	if secret != "" {
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
	}
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	return res
}

func TestWebhookAcceptsAnUpdateWithTheRightSecret(t *testing.T) {
	rec := newRecorder()
	mux := newServer(t, "s3cret", rec)

	res := post(t, mux, "s3cret", `{"update_id":1,"message":{"message_id":2,"text":"/start","chat":{"id":42,"type":"private"}}}`)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}

	select {
	case <-rec.done:
	case <-time.After(2 * time.Second):
		t.Fatal("update was never handled")
	}
	if rec.updates[0].Message.Text != "/start" {
		t.Fatalf("update = %+v", rec.updates[0])
	}
}

func TestWebhookRejectsAWrongSecret(t *testing.T) {
	rec := newRecorder()
	mux := newServer(t, "s3cret", rec)

	res := post(t, mux, "wrong", `{"update_id":1}`)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", res.Code)
	}
	if rec.count() != 0 {
		t.Fatal("a rejected update must not be handled")
	}
}

func TestWebhookRejectsAMissingSecretHeader(t *testing.T) {
	rec := newRecorder()
	mux := newServer(t, "s3cret", rec)

	if res := post(t, mux, "", `{"update_id":1}`); res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", res.Code)
	}
	if rec.count() != 0 {
		t.Fatal("a rejected update must not be handled")
	}
}

func TestWebhookFailsClosedWhenNoSecretIsConfigured(t *testing.T) {
	// Without a configured secret the route would accept anonymous posts, so
	// it refuses to serve at all rather than trusting whoever calls it.
	rec := newRecorder()
	mux := newServer(t, "", rec)

	if res := post(t, mux, "anything", `{"update_id":1}`); res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", res.Code)
	}
	if rec.count() != 0 {
		t.Fatal("no update may be handled without a configured secret")
	}
}

func TestWebhookRejectsAMalformedUpdate(t *testing.T) {
	rec := newRecorder()
	mux := newServer(t, "s3cret", rec)

	if res := post(t, mux, "s3cret", `{"update_id":`); res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.Code)
	}
}

func TestWebhookRejectsNonPost(t *testing.T) {
	mux := newServer(t, "s3cret", newRecorder())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/support/telegram/webhook", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", res.Code)
	}
}

func TestHealthAndSettingsServeOnBothPaths(t *testing.T) {
	mux := newServer(t, "s3cret", newRecorder())
	for _, path := range []string{"/health", "/api/v1/support/health", "/settings", "/api/v1/support/settings"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, res.Code)
		}
	}
}

func TestSettingsNeverCarriesASecret(t *testing.T) {
	mux := newServer(t, "s3cret", newRecorder())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/support/settings", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	if strings.Contains(res.Body.String(), "s3cret") {
		t.Fatalf("settings leaked the webhook secret: %s", res.Body.String())
	}
}
