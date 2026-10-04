package domain

import (
	"sort"
	"time"
)

const (
	SubscriptionStatusPending  = "pending"
	SubscriptionStatusAccepted = "accepted"
	SubscriptionStatusRejected = "rejected"
)

// TelegramSubscription is a Telegram user or chat registered for ops alerts.
type TelegramSubscription struct {
	ID             string `json:"id"`
	TelegramUserID *int64 `json:"telegram_user_id,omitempty"`
	ChatID         string `json:"chat_id"`
	ChatType       string `json:"chat_type,omitempty"`
	ChatLabel      string `json:"chat_label,omitempty"`
	Username       string `json:"username,omitempty"`
	Status         string `json:"status"`
	AlertOrder     bool   `json:"alert_order"`
	AlertProduct   bool   `json:"alert_product"`
	// AlertSupport opts a chat into customer inquiry handoffs from the
	// support bot. Separate from the order and product flags so staff can
	// watch the money path without also fielding consultations.
	AlertSupport bool `json:"alert_support"`
	// MutedEvents are messages this chat does not want inside a class it
	// receives — a chat on order alerts can drop "order.created" and keep
	// "order.paid". Always an array on the wire, empty when nothing is muted.
	MutedEvents []string   `json:"muted_events"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	AcceptedAt  *time.Time `json:"accepted_at,omitempty"`
	AcceptedBy  string     `json:"accepted_by,omitempty"`
}

func (s TelegramSubscription) IsAccepted() bool {
	return s.Status == SubscriptionStatusAccepted
}

// The messages a chat can mute, by alert class. They are the NATS subjects the
// dispatcher alerts on (shared/pkg/events); the domain spells them out because
// it imports nothing, and a service test keeps the two lists in step.
var (
	OrderAlertEvents = []string{
		"order.created",
		"order.paid",
		"order.status_updated",
		"payment.canceled",
		"payment.callback_rejected",
	}
	ProductAlertEvents = []string{
		"product.created",
		"product.updated",
		"product.deleted",
		"product.image_uploaded",
	}
)

// IsMutableEvent reports whether event is a message a chat may mute. Support
// handoffs are not: a shopper waiting on a person is the whole class.
func IsMutableEvent(event string) bool {
	for _, list := range [][]string{OrderAlertEvents, ProductAlertEvents} {
		for _, e := range list {
			if e == event {
				return true
			}
		}
	}
	return false
}

// NormalizeMutedEvents sorts and de-duplicates a muted list and reports the
// first name that is not a mutable event, so a typo is refused rather than
// stored as a mute that silences nothing.
func NormalizeMutedEvents(events []string) ([]string, string) {
	seen := make(map[string]struct{}, len(events))
	out := make([]string, 0, len(events))
	for _, e := range events {
		if !IsMutableEvent(e) {
			return nil, e
		}
		if _, ok := seen[e]; ok {
			continue
		}
		seen[e] = struct{}{}
		out = append(out, e)
	}
	sort.Strings(out)
	return out, ""
}

// Mutes reports whether the chat asked not to receive event.
func (s TelegramSubscription) Mutes(event string) bool {
	for _, e := range s.MutedEvents {
		if e == event {
			return true
		}
	}
	return false
}
