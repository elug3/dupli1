package telegram_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/notification/pkg/infra/telegram"
)

// Telegram answers getUpdates with 409 when a second consumer — another task,
// or a still-registered webhook — owns the update stream. The poller has to
// tell that apart from an ordinary failure so it can back off instead of
// retrying every few seconds for the length of a rolling deploy.
func TestGetUpdatesReportsConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":409,"description":"Conflict: terminated by other getUpdates request"}`))
	}))
	t.Cleanup(srv.Close)

	client := telegram.NewTestClient("test-token", srv.Client(), srv.URL)
	_, err := client.GetUpdates(t.Context(), 0, 0)
	if err == nil {
		t.Fatal("expected an error for a 409 response")
	}
	if !errors.Is(err, telegram.ErrUpdatesConflict) {
		t.Fatalf("expected ErrUpdatesConflict, got %v", err)
	}
}

// Any other non-2xx stays an ordinary error on the short backoff path.
func TestGetUpdatesOtherStatusIsNotConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	client := telegram.NewTestClient("test-token", srv.Client(), srv.URL)
	_, err := client.GetUpdates(t.Context(), 0, 0)
	if err == nil {
		t.Fatal("expected an error for a 500 response")
	}
	if errors.Is(err, telegram.ErrUpdatesConflict) {
		t.Fatalf("500 should not be reported as a conflict: %v", err)
	}
}
