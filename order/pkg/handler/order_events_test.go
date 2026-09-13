package handler_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/stream"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

// sseFrame is one parsed server-sent event.
type sseFrame struct {
	id      string
	event   string
	data    string
	comment bool
}

// readFrame reads lines up to the blank line that terminates one SSE frame.
func readFrame(t *testing.T, r *bufio.Reader) sseFrame {
	t.Helper()
	var frame sseFrame
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE frame: %v", err)
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case line == "":
			return frame
		case strings.HasPrefix(line, ":"):
			frame.comment = true
		case strings.HasPrefix(line, "id: "):
			frame.id = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			frame.event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			frame.data = strings.TrimPrefix(line, "data: ")
		case strings.HasPrefix(line, "retry: "):
			frame.event = "retry"
			frame.data = strings.TrimPrefix(line, "retry: ")
		}
	}
}

// openOrderStream connects to the live feed and returns the response plus a
// reader positioned after the opening retry frame — by which point the handler
// has registered with the hub, so a publish cannot race the subscription.
func openOrderStream(t *testing.T, srvURL, token, lastEventID string) (*http.Response, *bufio.Reader) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srvURL+"/api/v1/orders/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", bearerHeader(token))
	req.Header.Set("Accept", "text/event-stream")
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	reader := bufio.NewReader(res.Body)
	if frame := readFrame(t, reader); frame.event != "retry" {
		t.Fatalf("first frame = %+v, want the retry hint", frame)
	}
	return res, reader
}

func newStreamServer(t *testing.T) (*httptest.Server, *stream.Hub) {
	t.Helper()
	h, _ := newTestHandler(t)
	hub := stream.NewHub(0)
	srv := httptest.NewServer(newMux(h.WithOrderStream(hub)))
	t.Cleanup(srv.Close)
	return srv, hub
}

func adminToken(t *testing.T) string {
	t.Helper()
	return makeToken(t, "admin-1", []string{permissions.OrderReadAll})
}

// The stream must not become a way around the all-orders permission gate.
func TestOrderEvents_RequiresOrderReadAll(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h.WithOrderStream(stream.NewHub(0)))
	token := makeToken(t, "cust-1", []string{permissions.OrderCreate})

	w := do(t, mux, http.MethodGet, "/api/v1/orders/events", token, nil)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestOrderEvents_RequiresAuth(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h.WithOrderStream(stream.NewHub(0)))

	w := do(t, mux, http.MethodGet, "/api/v1/orders/events", "", nil)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

// Without NATS no order change ever reaches this task, so an open stream would
// be silently empty — a misconfiguration the caller should see.
func TestOrderEvents_WithoutHubReports503(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)

	w := do(t, mux, http.MethodGet, "/api/v1/orders/events", adminToken(t), nil)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestOrderEvents_RejectsNonGET(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h.WithOrderStream(stream.NewHub(0)))

	w := do(t, mux, http.MethodPost, "/api/v1/orders/events", adminToken(t), map[string]string{})

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}

// The route must win over the /api/v1/orders/{id} catch-all, or "events" would
// be read as an order id.
func TestOrderEvents_RouteIsNotShadowedByOrderIDLookup(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)

	w := do(t, mux, http.MethodGet, "/api/v1/orders/events", adminToken(t), nil)

	if w.Code == http.StatusNotFound {
		t.Fatal("stream route was shadowed by the order-id handler")
	}
}

// A stream that arrives only once it ends is the classic proxy-buffering
// failure; these headers are how the response opts out of it.
func TestOrderEvents_SendsUnbufferedStreamHeaders(t *testing.T) {
	srv, _ := newStreamServer(t)

	res, _ := openOrderStream(t, srv.URL, adminToken(t), "")

	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := res.Header.Get("Cache-Control"); !strings.Contains(cc, "no-transform") {
		t.Errorf("Cache-Control = %q, want it to contain no-transform", cc)
	}
	if ab := res.Header.Get("X-Accel-Buffering"); ab != "no" {
		t.Errorf("X-Accel-Buffering = %q, want \"no\"", ab)
	}
}

func TestOrderEvents_StreamsOrderSnapshotToConnectedClient(t *testing.T) {
	srv, hub := newStreamServer(t)
	_, reader := openOrderStream(t, srv.URL, adminToken(t), "")

	snapshot, err := json.Marshal(map[string]any{
		"type":  "order.created",
		"order": map[string]any{"id": "ord_live_1", "status": domain.StatusPending},
	})
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	hub.PublishOrderEvent("909", "order", snapshot)

	frame := readFrame(t, reader)
	if frame.id != "909" {
		t.Errorf("id = %q, want 909", frame.id)
	}
	if frame.event != "order" {
		t.Errorf("event = %q, want \"order\"", frame.event)
	}
	var body struct {
		Type  string `json:"type"`
		Order struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"order"`
	}
	if err := json.Unmarshal([]byte(frame.data), &body); err != nil {
		t.Fatalf("decode frame data %q: %v", frame.data, err)
	}
	if body.Type != "order.created" || body.Order.ID != "ord_live_1" {
		t.Errorf("payload = %+v, want the published order.created snapshot", body)
	}
}

// A cursor the hub cannot vouch for must produce a reset, or the admin table
// would sit missing whatever happened while the client was away.
func TestOrderEvents_StaleCursorGetsReset(t *testing.T) {
	srv, hub := newStreamServer(t)
	hub.PublishOrderEvent("500", "order", []byte(`{"type":"order.created"}`))

	_, reader := openOrderStream(t, srv.URL, adminToken(t), "100")

	frame := readFrame(t, reader)
	if frame.event != "reset" {
		t.Fatalf("first frame = %+v, want a reset", frame)
	}
}

func TestOrderEvents_LiveCursorReplaysMissedEvents(t *testing.T) {
	srv, hub := newStreamServer(t)
	hub.PublishOrderEvent("100", "order", []byte(`{"type":"order.created"}`))
	hub.PublishOrderEvent("200", "order", []byte(`{"type":"order.paid"}`))

	_, reader := openOrderStream(t, srv.URL, adminToken(t), "100")

	frame := readFrame(t, reader)
	if frame.event != "order" || frame.id != "200" {
		t.Fatalf("frame = %+v, want event 200 replayed", frame)
	}
}

// Dropping a lagging client is only safe if it is told to reload.
func TestOrderEvents_LaggingClientIsToldToResync(t *testing.T) {
	srv, hub := newStreamServer(t)
	_, reader := openOrderStream(t, srv.URL, adminToken(t), "")

	// Overflow the hub's per-client queue without reading from the socket. The
	// payload is padded so the socket buffer fills long before the publish loop
	// ends: while the handler can still write, it drains the queue and never
	// falls behind.
	fat := []byte(`{"type":"order.created","pad":"` + strings.Repeat("x", 4096) + `"}`)
	for range 512 {
		hub.PublishOrderEvent("909", "order", fat)
	}

	sawReset := false
	for range 512 {
		frame := readFrame(t, reader)
		if frame.event == "reset" {
			sawReset = true
			break
		}
	}
	if !sawReset {
		t.Fatal("dropped client never received a reset")
	}
}
