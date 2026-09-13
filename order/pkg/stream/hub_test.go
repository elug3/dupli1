package stream_test

import (
	"testing"

	"github.com/elug3/dupli1/order/pkg/stream"
)

// beyondClientBuffer exceeds the hub's per-client queue depth, so a client that
// never reads is guaranteed to overflow.
const beyondClientBuffer = 512

func TestHub_BroadcastsToEverySubscriber(t *testing.T) {
	hub := stream.NewHub(0)
	a, _, _ := hub.Subscribe("")
	defer a.Close()
	b, _, _ := hub.Subscribe("")
	defer b.Close()

	hub.PublishOrderEvent("100", "order", []byte(`{"type":"order.created"}`))

	for name, sub := range map[string]*stream.Subscription{"a": a, "b": b} {
		select {
		case ev := <-sub.C():
			if ev.ID != "100" || ev.Type != "order" {
				t.Fatalf("%s: got id=%q type=%q", name, ev.ID, ev.Type)
			}
		default:
			t.Fatalf("%s: no event delivered", name)
		}
	}
}

func TestHub_FreshClientGetsNoReplay(t *testing.T) {
	hub := stream.NewHub(0)
	hub.PublishOrderEvent("100", "order", []byte(`{}`))

	sub, replay, gapped := hub.Subscribe("")
	defer sub.Close()

	if gapped {
		t.Fatal("a client with no cursor has missed nothing; must not be told to resync")
	}
	if len(replay) != 0 {
		t.Fatalf("want no replay for a fresh client, got %d events", len(replay))
	}
}

func TestHub_ReplaysOnlyEventsAfterCursor(t *testing.T) {
	hub := stream.NewHub(0)
	hub.PublishOrderEvent("100", "order", []byte(`{"n":1}`))
	hub.PublishOrderEvent("200", "order", []byte(`{"n":2}`))
	hub.PublishOrderEvent("300", "order", []byte(`{"n":3}`))

	sub, replay, gapped := hub.Subscribe("200")
	defer sub.Close()

	if gapped {
		t.Fatal("cursor is inside the retained window; continuity holds")
	}
	if len(replay) != 1 || replay[0].ID != "300" {
		t.Fatalf("want only event 300 replayed, got %+v", replay)
	}
}

// A client away long enough for its cursor to fall out of the window cannot be
// caught up event-by-event — it has to reload from REST.
func TestHub_ReportsGapWhenCursorFellOutOfWindow(t *testing.T) {
	hub := stream.NewHub(2)
	hub.PublishOrderEvent("100", "order", []byte(`{}`))
	hub.PublishOrderEvent("200", "order", []byte(`{}`))
	hub.PublishOrderEvent("300", "order", []byte(`{}`))

	sub, _, gapped := hub.Subscribe("100")
	defer sub.Close()

	if !gapped {
		t.Fatal("event 200 was evicted after the client's cursor; want gapped")
	}
}

// A task that just started has no history, so it cannot prove the reconnecting
// client saw everything — the safe answer is resync, not silence.
func TestHub_ReportsGapWhenHistoryIsEmpty(t *testing.T) {
	hub := stream.NewHub(0)

	sub, replay, gapped := hub.Subscribe("100")
	defer sub.Close()

	if !gapped {
		t.Fatal("want gapped against an empty history")
	}
	if len(replay) != 0 {
		t.Fatalf("want no replay, got %d events", len(replay))
	}
}

func TestHub_ReportsGapOnUnparsableCursor(t *testing.T) {
	hub := stream.NewHub(0)
	hub.PublishOrderEvent("100", "order", []byte(`{}`))

	sub, _, gapped := hub.Subscribe("not-a-cursor")
	defer sub.Close()

	if !gapped {
		t.Fatal("want gapped for a cursor the hub cannot position")
	}
}

// A stalled reader must not be able to block delivery to everyone else.
func TestHub_DropsLaggingClientAndKeepsServingOthers(t *testing.T) {
	hub := stream.NewHub(0)
	slow, _, _ := hub.Subscribe("")
	defer slow.Close()

	for i := 0; i < beyondClientBuffer; i++ {
		hub.PublishOrderEvent("100", "order", []byte(`{}`))
	}

	// Drain the buffered events; the channel must then be closed, which is the
	// hub's signal that this client needs to resync.
	closed := false
	for range beyondClientBuffer + 1 {
		if _, ok := <-slow.C(); !ok {
			closed = true
			break
		}
	}
	if !closed {
		t.Fatal("want the lagging client's channel closed")
	}
	if hub.Subscribers() != 0 {
		t.Fatalf("want the lagging client unregistered, got %d subscribers", hub.Subscribers())
	}

	fresh, _, _ := hub.Subscribe("")
	defer fresh.Close()
	hub.PublishOrderEvent("600", "order", []byte(`{}`))
	select {
	case ev := <-fresh.C():
		if ev.ID != "600" {
			t.Fatalf("got id=%q, want 600", ev.ID)
		}
	default:
		t.Fatal("fan-out stopped working after dropping a lagging client")
	}
}

// Resets are live-only: replaying one would make a caught-up client reload for
// a failure it already recovered from.
func TestHub_ResetsAreBroadcastButNotRetained(t *testing.T) {
	hub := stream.NewHub(0)
	sub, _, _ := hub.Subscribe("")
	defer sub.Close()
	hub.PublishOrderEvent("100", "order", []byte(`{}`))

	hub.PublishReset("200")

	if _, ok := <-sub.C(); !ok {
		t.Fatal("order event not delivered")
	}
	ev, ok := <-sub.C()
	if !ok || ev.Type != "reset" {
		t.Fatalf("want a live reset, got %+v (open=%v)", ev, ok)
	}

	later, replay, gapped := hub.Subscribe("100")
	defer later.Close()
	if gapped {
		t.Fatal("cursor 100 is still in the window")
	}
	if len(replay) != 0 {
		t.Fatalf("want the reset not retained, got %+v", replay)
	}
}
