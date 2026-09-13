// Package stream fans order events out to live HTTP clients (SSE).
//
// Every order task subscribes to the order.* NATS subjects without a queue
// group, so each task receives every event and each task's history window holds
// the same events. That is what lets a browser reconnect to a different task
// and still replay from its Last-Event-ID.
package stream

import (
	"strconv"
	"sync"
)

// DefaultHistory is how many recent events a hub retains for reconnect replay.
// At admin order volumes this covers hours of disconnection.
const DefaultHistory = 256

// clientBuffer is the per-client queue depth. A client that falls further
// behind is dropped and told to resynchronize rather than stalling fan-out for
// everyone else.
const clientBuffer = 64

// Event is one server-sent event.
type Event struct {
	// ID is the SSE id line: a decimal nanosecond timestamp derived from the
	// source event, so every task computes the same id for the same event.
	// Empty means "unpositioned" — delivered, but it does not move the client's
	// cursor.
	ID string
	// Type is the SSE event name ("order", "reset").
	Type string
	// Data is the JSON payload. It must not contain literal newlines; JSON
	// escapes them inside strings, so marshalled output is always safe.
	Data []byte
}

// Subscription is one connected client's view of the hub.
type Subscription struct {
	hub *Hub
	id  int64
	ch  chan Event
}

// C is the event channel. It is closed when the hub drops this client for
// lagging — the caller must then tell the browser to resynchronize.
func (s *Subscription) C() <-chan Event { return s.ch }

// Close unregisters the subscription. Safe to call after the hub has already
// dropped it.
func (s *Subscription) Close() {
	if s == nil || s.hub == nil {
		return
	}
	s.hub.unsubscribe(s.id)
}

// Hub broadcasts events to subscribers and retains a short history so a
// reconnecting client can replay what it missed.
type Hub struct {
	mu      sync.Mutex
	subs    map[int64]chan Event
	nextID  int64
	history []Event
	histCap int
}

// NewHub returns a hub retaining historySize events (<= 0 uses DefaultHistory).
func NewHub(historySize int) *Hub {
	if historySize <= 0 {
		historySize = DefaultHistory
	}
	return &Hub{subs: make(map[int64]chan Event), histCap: historySize}
}

// Subscribe registers a client. replay holds the retained events newer than
// lastEventID; gapped reports that continuity from lastEventID could not be
// proven, so the caller must ask the client to resynchronize from scratch.
func (h *Hub) Subscribe(lastEventID string) (sub *Subscription, replay []Event, gapped bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	replay, gapped = h.replaySince(lastEventID)

	h.nextID++
	id := h.nextID
	ch := make(chan Event, clientBuffer)
	h.subs[id] = ch
	return &Subscription{hub: h, id: id, ch: ch}, replay, gapped
}

// replaySince must be called with the mutex held.
func (h *Hub) replaySince(lastEventID string) ([]Event, bool) {
	if lastEventID == "" {
		// A fresh client has just loaded the full list over REST; it needs no
		// replay and has missed nothing.
		return nil, false
	}
	since, err := strconv.ParseInt(lastEventID, 10, 64)
	if err != nil {
		return nil, true
	}
	if len(h.history) == 0 {
		// Restarted task (or one that has never seen an event): nothing proves
		// the client did not miss something.
		return nil, true
	}
	oldest, err := strconv.ParseInt(h.history[0].ID, 10, 64)
	if err != nil || oldest > since {
		// Events fell out of the window while the client was away.
		return nil, true
	}

	var replay []Event
	for _, ev := range h.history {
		evID, err := strconv.ParseInt(ev.ID, 10, 64)
		if err != nil || evID <= since {
			continue
		}
		replay = append(replay, ev)
	}
	return replay, false
}

// PublishOrderEvent retains an order event and broadcasts it to live clients.
func (h *Hub) PublishOrderEvent(id, eventType string, payload []byte) {
	h.publish(Event{ID: id, Type: eventType, Data: payload}, true)
}

// PublishReset tells every live client to reload from REST. Resets are not
// retained: a client that reconnects has already been told to resynchronize by
// the gap check, or is caught up.
func (h *Hub) PublishReset(id string) {
	h.publish(Event{ID: id, Type: "reset", Data: []byte(`{"reason":"resync"}`)}, false)
}

func (h *Hub) publish(ev Event, retain bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if retain && ev.ID != "" {
		h.history = append(h.history, ev)
		if len(h.history) > h.histCap {
			h.history = append(h.history[:0], h.history[len(h.history)-h.histCap:]...)
		}
	}

	for id, ch := range h.subs {
		select {
		case ch <- ev:
		default:
			// Lagging client: drop it. Closing the channel is the signal to
			// send a reset and end that stream; it reconnects on its own.
			close(ch)
			delete(h.subs, id)
		}
	}
}

func (h *Hub) unsubscribe(id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// The channel is closed only by publish when dropping a lagging client, so
	// deleting here never closes a channel the caller may still be reading.
	delete(h.subs, id)
}

// Subscribers reports the number of connected clients.
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}
