package domain

import "time"

// Inquiry status values.
const (
	InquiryOpen     = "open"
	InquiryAssigned = "assigned"
	InquiryAnswered = "answered"
	InquiryClosed   = "closed"
)

// Message directions.
const (
	DirectionInbound  = "inbound"
	DirectionOutbound = "outbound"
)

// Inquiry is one escalation: a shopper asked for a human.
type Inquiry struct {
	ID             string
	ConversationID string
	ChatID         string
	Topic          string
	Status         string
	AssignedTo     string
	OpenedAt       time.Time
	ClosedAt       *time.Time
}

// NewInquiry opens an inquiry on a topic.
func NewInquiry(id, conversationID, chatID, topic string, now time.Time) *Inquiry {
	return &Inquiry{
		ID:             id,
		ConversationID: conversationID,
		ChatID:         chatID,
		Topic:          topic,
		Status:         InquiryOpen,
		OpenedAt:       now,
	}
}

// IsOpen reports whether the inquiry is still waiting on staff.
func (i *Inquiry) IsOpen() bool {
	return i != nil && i.Status != InquiryClosed
}

// Message is one line of a conversation, kept as a business record.
//
// Bodies hold whatever a shopper typed — names, phone numbers, addresses — so
// they are retained for 180 days and then purged, and they never reach a log.
type Message struct {
	ID             string
	ConversationID string
	InquiryID      string
	Direction      string
	Author         string
	Body           string
	CreatedAt      time.Time
}

// Excerpt shortens a message for an ops alert.
//
// An alert is a doorbell: enough for a manager to decide whether to pick the
// inquiry up, not the conversation itself. The full text waits behind the
// manager inbox, which is authenticated.
func Excerpt(body string, limit int) string {
	runes := []rune(body)
	if len(runes) <= limit {
		return body
	}
	return string(runes[:limit]) + "…"
}
