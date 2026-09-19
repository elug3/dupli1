package ports

import (
	"context"
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"
)

// InquiryRepository stores escalations.
type InquiryRepository interface {
	// FindOpenByChatID returns the chat's unfinished inquiry, or nil.
	FindOpenByChatID(ctx context.Context, chatID string) (*domain.Inquiry, error)
	FindByID(ctx context.Context, id string) (*domain.Inquiry, error)
	// List returns inquiries for the inbox, newest first.
	List(ctx context.Context, filter InquiryFilter) ([]domain.Inquiry, error)
	// ListStaleOpen returns inquiries with no activity since the cutoff.
	ListStaleOpen(ctx context.Context, quietSince time.Time) ([]domain.Inquiry, error)
	Save(ctx context.Context, inquiry *domain.Inquiry) error
}

// InquiryFilter narrows an inbox listing.
//
// Status and AssignedTo are separate because the inbox has three lists — 대기,
// 내 상담, 완료 — and "mine" is a different question from "open".
type InquiryFilter struct {
	Status     string
	AssignedTo string
	// Unassigned selects the 대기 queue: open and claimed by nobody.
	Unassigned bool
	Limit      int
}

// MessageRepository stores what was said, in both directions.
type MessageRepository interface {
	Append(ctx context.Context, message *domain.Message) error
	// Transcript returns a conversation's messages, oldest first.
	Transcript(ctx context.Context, conversationID string) ([]domain.Message, error)
}

// MessageReader is implemented by message stores that can look back. It is a
// separate interface so a store without it still satisfies MessageRepository —
// the excerpt is a nicety in an alert, not something to fail an escalation for.
type MessageReader interface {
	// LastInbound returns the most recent thing the shopper typed, or "".
	LastInbound(ctx context.Context, conversationID string) (string, error)
}

// InquiryPublisher announces an escalation to the rest of the platform.
//
// Support does not deliver ops alerts itself: notification owns which chats
// receive them, including the accepted-subscription rows and the per-chat alert
// flags. Publishing keeps one owner of that routing and keeps the ops bot's
// credentials out of the customer-facing service.
type InquiryPublisher interface {
	InquiryOpened(ctx context.Context, inquiry InquiryOpened) error
}

// InquiryOpened is what support announces when a shopper asks for a human.
type InquiryOpened struct {
	InquiryID    string
	ChatID       string
	Topic        string
	Language     string
	Username     string
	EntryContext string
	Excerpt      string
	AfterHours   bool
}
