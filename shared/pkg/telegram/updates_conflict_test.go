package telegram_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elug3/dupli1/shared/pkg/telegram"
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

// The batch limit has to reach Telegram, otherwise it defaults to 100 updates
// and a busy backlog can outgrow any read budget in a single response.
func TestGetUpdatesRequestsABoundedBatch(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	t.Cleanup(srv.Close)

	client := telegram.NewTestClient("test-token", srv.Client(), srv.URL)
	if _, err := client.GetUpdates(t.Context(), 7, 30); err != nil {
		t.Fatalf("GetUpdates: %v", err)
	}
	for _, want := range []string{"offset=7", "timeout=30", "limit="} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query %q is missing %q", gotQuery, want)
		}
	}
}

// An oversized body is reported as itself. Silently truncating it produced a
// JSON decode error on every poll, and since the offset only advances on a
// successful decode, polling stalled for good.
func TestGetUpdatesRejectsAnOversizedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":[`))
		// Far past the read budget, and never validly terminated.
		chunk := strings.Repeat("x", 64<<10)
		for written := 0; written < (5 << 20); written += len(chunk) {
			if _, err := w.Write([]byte(chunk)); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)

	client := telegram.NewTestClient("test-token", srv.Client(), srv.URL)
	_, err := client.GetUpdates(t.Context(), 0, 0)
	if err == nil {
		t.Fatal("expected an error for an oversized response")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error should name the size limit, got %v", err)
	}
}
