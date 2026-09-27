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

	"github.com/elug3/dupli1/order/pkg/handler"
	"github.com/elug3/dupli1/order/pkg/infra/memory"
	"github.com/elug3/dupli1/order/pkg/livefeed"
	"github.com/elug3/dupli1/order/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/golang-jwt/jwt/v5"
)

type sseEvent struct {
	ID, Event, Data string
}

// streamServer wires the hub the way bootstrap does without a broker, behind a
// real server whose WriteTimeout is far shorter than any stream.
func streamServer(t *testing.T) (*httptest.Server, *service.Service) {
	t.Helper()
	var svc *service.Service
	hub := livefeed.NewHub(func(ctx context.Context, id string) (any, error) {
		return svc.GetOrder(ctx, id)
	}, livefeed.DefaultBufferSize)
	svc = service.New(memory.NewRepository(), &fakeStock{}, livefeed.Publisher{Hub: hub}).
		WithProduct(&fakeProduct{price: 1000})
	h := handler.New(svc, authjwt.NewHMACValidator(testSecret)).WithOrderFeed(hub)

	srv := httptest.NewUnstartedServer(newMux(h))
	srv.Config.WriteTimeout = time.Second
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, svc
}

func tokenExpiringIn(t *testing.T, perms []string, ttl time.Duration) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":         "mgr-1",
		"type":        "access",
		"permissions": perms,
		"exp":         time.Now().Add(ttl).Unix(),
	})
	signed, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// openStream connects and returns parsed events until the stream ends.
func openStream(t *testing.T, srv *httptest.Server, token, lastEventID string) (*http.Response, <-chan sseEvent) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/orders/events", nil)
	req.Header.Set("Authorization", bearerHeader(token))
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	out := make(chan sseEvent, 32)
	go func() {
		defer close(out)
		defer res.Body.Close()
		scanner := bufio.NewScanner(res.Body)
		var ev sseEvent
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == "":
				if ev != (sseEvent{}) {
					out <- ev
				}
				ev = sseEvent{}
			case strings.HasPrefix(line, "id: "):
				ev.ID = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "event: "):
				ev.Event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				ev.Data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	return res, out
}

// nextOrder skips non-order events and returns the next order frame.
func nextOrder(t *testing.T, events <-chan sseEvent) (sseEvent, string, map[string]any) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("stream ended")
			}
			if ev.Event != "order" {
				continue
			}
			var frame struct {
				Type  string         `json:"type"`
				Order map[string]any `json:"order"`
			}
			if err := json.Unmarshal([]byte(ev.Data), &frame); err != nil {
				t.Fatalf("frame %q: %v", ev.Data, err)
			}
			return ev, frame.Type, frame.Order
		case <-deadline:
			t.Fatal("no order frame")
		}
	}
}

func TestOrderEvents_RequiresOrderReadAll(t *testing.T) {
	srv, _ := streamServer(t)
	res, _ := openStream(t, srv, tokenExpiringIn(t, nil, time.Hour), "")
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a customer token", res.StatusCode)
	}
}

// The order moves through its lifecycle; each change reaches the stream as a
// snapshot matching GET /orders/{id}, well past the server's WriteTimeout.
func TestOrderEvents_StreamsSnapshotsOfEachChange(t *testing.T) {
	srv, svc := streamServer(t)
	res, events := openStream(t, srv, tokenExpiringIn(t, []string{permissions.OrderReadAll}, time.Hour), "")
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status=%d type=%q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	if res.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatal("missing X-Accel-Buffering: no — nginx would buffer the stream")
	}

	time.Sleep(1500 * time.Millisecond) // past the 1s WriteTimeout

	id := seedPaidOrder(t, svc, "cust-1")
	_, typ, order := nextOrder(t, events)
	if typ != "order.created" || order["id"] != id || order["status"] != "pending" {
		t.Fatalf("first frame: type=%s order=%v", typ, order)
	}
	_, typ, order = nextOrder(t, events)
	if typ != "order.paid" || order["status"] != "paid" {
		t.Fatalf("second frame: type=%s status=%v", typ, order["status"])
	}
	if order["confirmation_due_at"] == nil {
		t.Fatal("snapshot lacks confirmation_due_at — not the presented order")
	}

	if _, err := svc.ConfirmOrder(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	for {
		_, typ, order = nextOrder(t, events)
		if order["status"] == "confirmed" {
			break
		}
	}
	if typ != "order.status_updated" {
		t.Fatalf("confirm arrived as %s", typ)
	}
}

func TestOrderEvents_ReconnectReplaysOrResets(t *testing.T) {
	srv, svc := streamServer(t)
	token := tokenExpiringIn(t, []string{permissions.OrderReadAll}, time.Hour)

	_, events := openStream(t, srv, token, "")
	seedOrder(t, svc, "cust-1")
	first, _, _ := nextOrder(t, events)
	seedOrder(t, svc, "cust-2") // missed by a client that stops after the first

	_, replayed := openStream(t, srv, token, first.ID)
	_, _, order := nextOrder(t, replayed)
	if order["customer_id"] != "cust-2" {
		t.Fatalf("replay = %v, want the missed cust-2 order", order)
	}

	_, stale := openStream(t, srv, token, "42") // from an earlier process
	select {
	case ev := <-stale:
		for ev.Event == "" { // retry: line
			ev = <-stale
		}
		if ev.Event != "reset" {
			t.Fatalf("stale cursor got %q, want reset", ev.Event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no reset")
	}
}

// The token is only checked when the stream opens, so the stream must end
// when it expires; the browser reconnects with a fresh one from the BFF.
func TestOrderEvents_EndsWhenTheTokenExpires(t *testing.T) {
	srv, _ := streamServer(t)
	_, events := openStream(t, srv, tokenExpiringIn(t, []string{permissions.OrderReadAll}, 2*time.Second), "")
	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("stream still open after its token expired")
		}
	}
}
