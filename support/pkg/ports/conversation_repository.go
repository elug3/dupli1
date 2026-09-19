// Package ports declares the interfaces the service depends on. Infra
// implements them; the service never sees a database or an HTTP client.
package ports

import (
	"context"

	"github.com/elug3/dupli1/support/pkg/domain"
)

// ConversationRepository stores one row per Telegram chat.
type ConversationRepository interface {
	// FindByChatID returns nil (and no error) when the chat is unknown.
	FindByChatID(ctx context.Context, chatID string) (*domain.Conversation, error)
	Save(ctx context.Context, conversation *domain.Conversation) error
}

// Bot is the outbound half of a Telegram conversation.
//
// Declared here rather than taken as *telegram.Client so the service layer can
// be tested without a Bot API server, and so the transport stays swappable.
type Bot interface {
	// ReplyMenu sends text with an inline keyboard of (label, callbackData).
	ReplyMenu(ctx context.Context, chatID string, text string, buttons []MenuButton) error
	// EditMenu replaces an earlier message's text and buttons, which is how a
	// menu walks in place instead of stacking one message per tap.
	EditMenu(ctx context.Context, chatID string, messageID int64, text string, buttons []MenuButton) error
	// AnswerCallback dismisses the loading spinner on a tapped button.
	AnswerCallback(ctx context.Context, callbackQueryID string, text string) error
}

// MenuButton is one inline-keyboard button, already rendered for the wire.
type MenuButton struct {
	Label        string
	CallbackData string
}
