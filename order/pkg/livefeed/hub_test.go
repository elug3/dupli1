package livefeed

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/elug3/dupli1/shared/pkg/events"
)

func payload(t *testing.T, subject, orderID string) []byte {
	t.Helper()
	b, err := json.Marshal(events.Order{EventType: subject, OrderID: orderID})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func snapshotOf(status string) Snapshot {
	return func(_ context.Context, id string) (any, error) {
		return map[string]string{"id": id, "status": status}, nil
	}
}

func recv(t *testing.T, sub *Subscription) Frame {
	t.Helper()
	select {
	case f, ok := <-sub.C:
		if !ok {
			t.Fatal("subscription closed")
		}
		return f
	case <-time.After(time.Second):
		t.Fatal("no frame")
	}
	return Frame{}
}

func TestNotifySendsTheCurrentSnapshotToEveryStream(t *testing.T) {
	h := NewHub(snapshotOf("paid"), 8)
	a, _, _ := h.Subscribe("")
	b, _, _ := h.Subscribe("")

	h.Notify(t.Context(), events.OrderPaid, payload(t, events.OrderPaid, "ord_1"))

	for _, sub := range []*Subscription{a, b} {
		var got struct {
			Type  string            `json:"type"`
			Order map[string]string `json:"order"`
		}
		if err := json.Unmarshal(recv(t, sub).Data, &got); err != nil {
			t.Fatal(err)
		}
		if got.Type != events.OrderPaid || got.Order["id"] != "ord_1" || got.Order["status"] != "paid" {
			t.Fatalf("frame = %+v", got)
		}
	}
}

func TestNotifyIgnoresOtherSubjectsAndUnloadableOrders(t *testing.T) {
	h := NewHub(func(context.Context, string) (any, error) { return nil, errors.New("gone") }, 8)
	sub, _, _ := h.Subscribe("")
	h.Notify(t.Context(), "payment.succeeded", payload(t, "payment.succeeded", "ord_1"))
	h.Notify(t.Context(), events.OrderCreated, []byte("not json"))
	h.Notify(t.Context(), events.OrderCreated, payload(t, events.OrderCreated, "ord_1")) // load fails
	select {
	case f := <-sub.C:
		t.Fatalf("unexpected frame %s", f.Data)
	default:
	}
}

func TestReconnectReplaysWhatWasMissed(t *testing.T) {
	h := NewHub(snapshotOf("paid"), 8)
	first, _, _ := h.Subscribe("")
	h.Publish(events.OrderCreated, "a")
	seen := recv(t, first)
	h.Unsubscribe(first)

	h.Publish(events.OrderPaid, "b")
	h.Publish(events.OrderStatusUpdate, "c")

	_, replay, reset := h.Subscribe(seen.ID)
	if reset || len(replay) != 2 {
		t.Fatalf("replay=%d reset=%v, want the 2 missed frames", len(replay), reset)
	}
	// Caught up: nothing to replay.
	_, replay, reset = h.Subscribe(replay[1].ID)
	if reset || len(replay) != 0 {
		t.Fatalf("caught-up cursor: replay=%d reset=%v", len(replay), reset)
	}
}

func TestReconnectResetsWhenTheCursorCannotBeVouchedFor(t *testing.T) {
	h := NewHub(snapshotOf("paid"), 2)
	for i := 0; i < 5; i++ {
		h.Publish(events.OrderCreated, i)
	}
	for _, cursor := range []string{
		"1",                    // from an earlier process (ids start at start-up time)
		"garbage",              // not ours at all
		"99999999999999999999", // from the future
	} {
		if _, _, reset := h.Subscribe(cursor); !reset {
			t.Errorf("cursor %q: want reset", cursor)
		}
	}
	// Older than the ring (it holds only the last 2 of 5).
	old := h.base + 1
	if _, _, reset := h.Subscribe(itoa(old)); !reset {
		t.Error("cursor older than the ring: want reset")
	}
}

func TestSlowStreamIsDroppedInsteadOfBlockingOthers(t *testing.T) {
	h := NewHub(snapshotOf("paid"), DefaultBufferSize)
	slow, _, _ := h.Subscribe("")
	fast, _, _ := h.Subscribe("")
	for i := 0; i < subscriberQueue+1; i++ {
		h.Publish(events.OrderCreated, i)
		recv(t, fast) // keeps up
	}
	// The slow one's channel was closed once it overflowed.
	n := 0
	for range slow.C {
		n++
	}
	if n != subscriberQueue {
		t.Fatalf("slow stream got %d frames before being dropped, want %d", n, subscriberQueue)
	}
	if h.Subscribers() != 1 {
		t.Fatalf("subscribers = %d, want only the fast one", h.Subscribers())
	}
	h.Unsubscribe(slow) // already dropped: must not panic
	h.Unsubscribe(fast)
}

func itoa(n uint64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
