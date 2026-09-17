package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// The retry backoff is real time nobody needs to spend in a unit test; the
// attempt counts below still exercise the full budget.
func TestMain(m *testing.M) {
	sendRetryBackoff = time.Millisecond
	os.Exit(m.Run())
}

func countingServer(t *testing.T, status func(attempt int32) int) (*Client, *int32) {
	t.Helper()
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		code := status(n)
		w.WriteHeader(code)
		if code >= 200 && code < 300 {
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	t.Cleanup(srv.Close)
	return NewTestClient("test-token", srv.Client(), srv.URL), &attempts
}

// Core NATS does not redeliver, so a send that fails on a blip loses the alert
// outright — order.paid included. A transient 5xx must be retried.
func TestSendRetriesServerErrors(t *testing.T) {
	client, attempts := countingServer(t, func(n int32) int {
		if n < 3 {
			return http.StatusBadGateway
		}
		return http.StatusOK
	})

	if err := client.Send(context.Background(), "-100123", "hello"); err != nil {
		t.Fatalf("Send should have succeeded on the third attempt: %v", err)
	}
	if got := atomic.LoadInt32(attempts); got != 3 {
		t.Fatalf("attempts = %d, want 3", got)
	}
}

func TestSendRetriesRateLimits(t *testing.T) {
	client, attempts := countingServer(t, func(n int32) int {
		if n < 2 {
			return http.StatusTooManyRequests
		}
		return http.StatusOK
	})

	if err := client.Send(context.Background(), "-100123", "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := atomic.LoadInt32(attempts); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
}

// A rejected chat or a malformed message fails identically every time; retrying
// only delays the NATS handler behind it.
func TestSendDoesNotRetryClientErrors(t *testing.T) {
	client, attempts := countingServer(t, func(int32) int { return http.StatusBadRequest })

	if err := client.Send(context.Background(), "-100123", "hello"); err == nil {
		t.Fatal("expected a 400 to fail")
	}
	if got := atomic.LoadInt32(attempts); got != 1 {
		t.Fatalf("attempts = %d, want a single attempt", got)
	}
}

func TestSendGivesUpAfterTheAttemptBudget(t *testing.T) {
	client, attempts := countingServer(t, func(int32) int { return http.StatusInternalServerError })

	if err := client.Send(context.Background(), "-100123", "hello"); err == nil {
		t.Fatal("expected the send to fail once the budget is spent")
	}
	if got := atomic.LoadInt32(attempts); got != int32(sendMaxAttempts) {
		t.Fatalf("attempts = %d, want %d", got, sendMaxAttempts)
	}
}

// A cancelled context must not keep the handler waiting out the backoff.
func TestSendStopsRetryingOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client, attempts := countingServer(t, func(int32) int {
		cancel()
		return http.StatusInternalServerError
	})

	if err := client.Send(ctx, "-100123", "hello"); err == nil {
		t.Fatal("expected an error")
	}
	if got := atomic.LoadInt32(attempts); got != 1 {
		t.Fatalf("attempts = %d, want a single attempt before the context ended", got)
	}
}
