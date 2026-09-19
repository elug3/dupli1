package ports

import (
	"context"

	"github.com/elug3/dupli1/support/pkg/domain"
)

// InquiryRepository stores escalations.
type InquiryRepository interface {
	// FindOpenByChatID returns the chat's unfinished inquiry, or nil.
	FindOpenByChatID(ctx context.Context, chatID string) (*domain.Inquiry, error)
	Save(ctx context.Context, inquiry *domain.Inquiry) error
}

// MessageRepository stores what was said, in both directions.
type MessageRepository interface {
	Append(ctx context.Context, message *domain.Message) error
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
