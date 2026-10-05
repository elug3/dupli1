package sentrymon

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getsentry/sentry-go"
)

func initForTest(t *testing.T) *sentry.MockTransport {
	t.Helper()
	mock := &sentry.MockTransport{}
	transport = mock
	t.Setenv("SENTRY_DSN", "https://public@sentry.example.com/1")
	t.Setenv("SENTRY_LOGS", "false")
	Init("test")
	t.Cleanup(func() {
		enabled = false
		transport = nil
		sentry.CurrentHub().BindClient(nil)
	})
	return mock
}

func TestDisabledWithoutDSN(t *testing.T) {
	t.Setenv("SENTRY_DSN", "")
	Init("test")
	if Enabled() {
		t.Fatal("enabled without SENTRY_DSN")
	}
	h := http.NotFoundHandler()
	if got := Handler(h); got == nil {
		t.Fatal("nil handler")
	}
	rec := httptest.NewRecorder()
	Handler(h).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestReports5xxNot4xx(t *testing.T) {
	mock := initForTest(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == "boom" {
			http.Error(w, "x", http.StatusBadGateway)
			return
		}
		http.Error(w, "x", http.StatusNotFound)
	})
	h := Handler(mux)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/orders/missing", nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/orders/boom", nil))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d", rec.Code)
	}

	events := mock.Events()
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if want := "HTTP 502 GET /orders/{id}"; events[0].Message != want {
		t.Fatalf("message = %q, want %q", events[0].Message, want)
	}
	if events[0].Tags["service"] != "test" {
		t.Fatalf("service tag = %q", events[0].Tags["service"])
	}
}

func TestPanicIsReportedAndRepanics(t *testing.T) {
	mock := initForTest(t)
	h := Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("kaboom") }))
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("panic was swallowed")
			}
		}()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	}()
	events := mock.Events()
	if len(events) != 1 || !strings.Contains(events[0].Message, "kaboom") {
		t.Fatalf("panic not reported: %+v", events)
	}
}

func TestStreamingStillFlushes(t *testing.T) {
	initForTest(t)
	var flushed bool
	h := Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, flushed = w.(http.Flusher)
		_ = http.NewResponseController(w).Flush()
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/events", nil))
	if !flushed {
		t.Fatal("handler lost http.Flusher")
	}
}

func TestLevelOf(t *testing.T) {
	cases := map[string]Level{
		"order abc: publish failed":   LevelError,
		"telegram drain updates: ERR": LevelInfo,
		"warning: slow query":         LevelWarn,
		"Starting order server":       LevelInfo,
	}
	for msg, want := range cases {
		if got := levelOf(msg); got != want {
			t.Errorf("levelOf(%q) = %v, want %v", msg, got, want)
		}
	}
}
