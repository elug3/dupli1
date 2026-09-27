// Package livefeed fans order changes out to open admin event streams
// (GET /api/v1/orders/events, see docs/order-live-events.md).
//
// The hub is fed from the order outbox — every order state change, manual or
// by an SLA sweep, writes an outbox row — either by a broadcast NATS
// subscription (each replica sees every change, so a stream on any replica is
// complete) or, with no broker configured, by the drainer's publish step.
//
// Each change becomes one frame holding the order exactly as
// GET /api/v1/orders/{id} returns it, so a client never refetches. Frames are
// kept in a bounded ring so a client reconnecting with Last-Event-ID gets what
// it missed; one whose cursor is older than the ring, or from before this
// process started, is told to reset and reload from REST instead.
package livefeed

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/elug3/dupli1/shared/pkg/events"
)

// DefaultBufferSize is how many recent frames a reconnecting client can
// replay. At a few orders a minute this is hours of history.
const DefaultBufferSize = 512

// subscriberQueue is how far a client may fall behind before it is dropped.
// A dropped client reconnects with its cursor and replays from the ring.
const subscriberQueue = 64

// Snapshot loads an order as the REST API presents it.
type Snapshot func(ctx context.Context, orderID string) (any, error)

// Frame is one `event: order` SSE message.
type Frame struct {
	ID   string
	Data []byte
}

// Subscription is one open stream. C is closed when the hub drops the client
// for falling behind; the stream should end so the client reconnects.
type Subscription struct {
	C chan Frame
}

// Hub is safe for concurrent use.
type Hub struct {
	snapshot Snapshot
	size     int

	mu   sync.Mutex
	base uint64 // ids start here, so a cursor from an earlier process is never mistaken for one of ours
	next uint64
	ring []Frame // oldest first, at most size
	subs map[*Subscription]struct{}
}

// NewHub builds a hub that loads snapshots with snapshot.
func NewHub(snapshot Snapshot, size int) *Hub {
	if size <= 0 {
		size = DefaultBufferSize
	}
	base := uint64(time.Now().UnixNano())
	return &Hub{
		snapshot: snapshot,
		size:     size,
		base:     base,
		next:     base,
		subs:     make(map[*Subscription]struct{}),
	}
}

// relayed are the subjects that reach the stream (shared/pkg/events).
var relayed = map[string]bool{
	events.OrderCreated:      true,
	events.OrderPaid:         true,
	events.OrderStatusUpdate: true,
}

// Notify handles one outbox event: load the order's current state and send it
// to every open stream. payload is the outbox JSON (events.Order).
func (h *Hub) Notify(ctx context.Context, subject string, payload []byte) {
	if !relayed[subject] {
		return
	}
	var ev events.Order
	if err := json.Unmarshal(payload, &ev); err != nil || ev.OrderID == "" {
		log.Printf("livefeed: skip %s: unreadable payload", subject)
		return
	}
	order, err := h.snapshot(ctx, ev.OrderID)
	if err != nil {
		log.Printf("livefeed: load order %s for %s: %v", ev.OrderID, subject, err)
		return
	}
	h.Publish(subject, order)
}

// Publish sends one order snapshot to every open stream.
func (h *Hub) Publish(subject string, order any) {
	data, err := json.Marshal(struct {
		Type  string `json:"type"`
		Order any    `json:"order"`
	}{subject, order})
	if err != nil {
		log.Printf("livefeed: marshal %s: %v", subject, err)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.next++
	frame := Frame{ID: strconv.FormatUint(h.next, 10), Data: data}
	h.ring = append(h.ring, frame)
	if len(h.ring) > h.size {
		h.ring = append(h.ring[:0:0], h.ring[len(h.ring)-h.size:]...)
	}
	for sub := range h.subs {
		select {
		case sub.C <- frame:
		default:
			// Too far behind: drop it rather than block every other stream.
			delete(h.subs, sub)
			close(sub.C)
		}
	}
}

// Subscribe opens a stream. With a Last-Event-ID the hub still holds, replay
// is what the client missed; with one it cannot vouch for (too old, or from
// before this process started), reset is true and the client must reload.
// A fresh client (empty lastEventID) gets neither.
func (h *Hub) Subscribe(lastEventID string) (sub *Subscription, replay []Frame, reset bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if lastEventID != "" {
		cursor, err := strconv.ParseUint(lastEventID, 10, 64)
		// The earliest cursor we can continue from is the id just before the
		// oldest frame still held (or our base when nothing was dropped yet).
		earliest := h.base
		if len(h.ring) > 0 {
			first, _ := strconv.ParseUint(h.ring[0].ID, 10, 64)
			earliest = first - 1
		}
		switch {
		case err != nil || cursor < earliest || cursor > h.next:
			reset = true
		default:
			for _, frame := range h.ring {
				id, _ := strconv.ParseUint(frame.ID, 10, 64)
				if id > cursor {
					replay = append(replay, frame)
				}
			}
		}
	}

	sub = &Subscription{C: make(chan Frame, subscriberQueue)}
	h.subs[sub] = struct{}{}
	return sub, replay, reset
}

// Unsubscribe closes a stream. Safe after the hub already dropped it.
func (h *Hub) Unsubscribe(sub *Subscription) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[sub]; ok {
		delete(h.subs, sub)
		close(sub.C)
	}
}

// Subscribers reports how many streams are open (settings, tests).
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}
