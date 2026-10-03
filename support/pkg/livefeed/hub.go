// Package livefeed fans consultation changes out to open chat streams: a web
// shopper's own stream (GET /api/v1/support/web/events) and the manager
// inbox's (GET /api/v1/support/inquiries/events). See docs/support-web-chat.md.
//
// A frame names what changed — an inquiry, a message — never what was said.
// Clients reload through the authenticated REST API on each frame, so a
// dropped frame or a reconnect costs freshness, not correctness, and the hub
// needs no replay ring.
package livefeed

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/elug3/dupli1/support/pkg/ports"
)

// subscriberQueue is how far a client may fall behind before it is dropped.
// A dropped client reconnects and reloads.
const subscriberQueue = 32

// Frame is one SSE message: `event: <Event>` with JSON Data.
type Frame struct {
	Event string
	Data  []byte
}

// Subscription is one open stream. C is closed when the hub drops the client
// for falling behind; the stream should then end so the client reconnects.
type Subscription struct {
	C chan Frame

	customerID string
	inbox      bool
}

// Hub is safe for concurrent use.
type Hub struct {
	mu   sync.Mutex
	subs map[*Subscription]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[*Subscription]struct{})}
}

// SubscribeCustomer opens a stream that hears only that customer's own
// conversation.
func (h *Hub) SubscribeCustomer(customerID string) *Subscription {
	return h.subscribe(&Subscription{C: make(chan Frame, subscriberQueue), customerID: customerID})
}

// SubscribeInbox opens a stream that hears every consultation, for staff.
func (h *Hub) SubscribeInbox() *Subscription {
	return h.subscribe(&Subscription{C: make(chan Frame, subscriberQueue), inbox: true})
}

func (h *Hub) subscribe(sub *Subscription) *Subscription {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subs[sub] = struct{}{}
	return sub
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

// Subscribers reports how many streams are open (tests).
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// customerFrame is what a shopper's stream carries: enough to know to reload,
// nothing about staff (no manager id, no other conversation).
type customerFrame struct {
	Type      string `json:"type"`
	InquiryID string `json:"inquiry_id,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	Status    string `json:"status,omitempty"`
}

// inboxFrame is what the staff stream carries.
type inboxFrame struct {
	Type           string `json:"type"`
	InquiryID      string `json:"inquiry_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	Channel        string `json:"channel,omitempty"`
	MessageID      string `json:"message_id,omitempty"`
	Status         string `json:"status,omitempty"`
}

// PublishLive delivers one change to the streams on this process that may
// hear it: every inbox stream, and the stream of the customer it concerns.
func (h *Hub) PublishLive(_ context.Context, event ports.LiveEvent) {
	if h == nil || event.Type == "" {
		return
	}
	customerData, err := json.Marshal(customerFrame{
		Type: event.Type, InquiryID: event.InquiryID, MessageID: event.MessageID, Status: event.Status,
	})
	if err != nil {
		return
	}
	inboxData, err := json.Marshal(inboxFrame{
		Type: event.Type, InquiryID: event.InquiryID, ConversationID: event.ConversationID,
		Channel: event.Channel, MessageID: event.MessageID, Status: event.Status,
	})
	if err != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs {
		var frame Frame
		switch {
		case sub.inbox:
			frame = Frame{Event: event.Type, Data: inboxData}
		case event.CustomerID != "" && sub.customerID == event.CustomerID:
			frame = Frame{Event: event.Type, Data: customerData}
		default:
			continue
		}
		select {
		case sub.C <- frame:
		default:
			// Too far behind: drop it rather than block every other stream.
			delete(h.subs, sub)
			close(sub.C)
		}
	}
}
