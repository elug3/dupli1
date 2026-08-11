package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/elug3/dupli1/notification/pkg/domain"
)

// SubscriptionLookup resolves stored Telegram subscriptions for inbound messages.
type SubscriptionLookup interface {
	RegisterFromMessage(ctx context.Context, in SubscriptionInput) (*domain.TelegramSubscription, error)
	FindForMessage(ctx context.Context, chatID string, userID *int64) (*domain.TelegramSubscription, error)
}

// SubscriptionInput captures fields from an inbound Telegram message.
type SubscriptionInput struct {
	TelegramUserID *int64
	ChatID         string
	ChatType       string
	ChatLabel      string
	Username       string
}

// UpdateProcessor handles Telegram updates from webhook or getUpdates.
type UpdateProcessor struct {
	Client   *Client
	Lookup   SubscriptionLookup
	Policy   AccessPolicy
}

func (p *UpdateProcessor) Handle(ctx context.Context, update Update) error {
	if p == nil || update.Message == nil {
		return nil
	}
	msg := update.Message

	in := SubscriptionInput{
		ChatID:   msg.Chat.FormatID(),
		ChatType: msg.Chat.Type,
		ChatLabel: chatLabelText(msg.Chat),
	}
	if msg.From != nil {
		id := msg.From.ID
		in.TelegramUserID = &id
		in.Username = msg.From.Username
	}

	var sub *domain.TelegramSubscription
	if p.Lookup != nil {
		var err error
		sub, err = p.Lookup.RegisterFromMessage(ctx, in)
		if err != nil {
			return fmt.Errorf("register telegram subscription: %w", err)
		}
	}

	if !IsStartCommand(msg.Text) {
		return nil
	}

	return p.replyStart(ctx, msg, sub)
}

func (p *UpdateProcessor) replyStart(ctx context.Context, msg *Message, sub *domain.TelegramSubscription) error {
	if p.Client == nil {
		return nil
	}

	if sub != nil && sub.Status == domain.SubscriptionStatusPending {
		// Pending chats are not outbound-allowlisted yet; Reply bypasses AllowsChat.
		return p.Client.Reply(ctx, msg.Chat.FormatID(), FormatPendingReply(msg.Chat))
	}

	allowed := sub != nil && sub.IsAccepted()
	if !allowed && p.Policy != nil {
		allowed = p.Policy.AllowsIncoming(msg.Chat, msg.From)
	}
	if !allowed {
		return nil
	}

	return p.Client.Reply(ctx, msg.Chat.FormatID(), FormatStartReply(msg.Chat))
}

func chatLabelText(chat Chat) string {
	switch strings.TrimSpace(chat.Type) {
	case "private":
		if name := strings.TrimSpace(chat.FirstName); name != "" {
			return name
		}
		return strings.TrimSpace(chat.Username)
	case "group", "supergroup", "channel":
		return strings.TrimSpace(chat.Title)
	default:
		return ""
	}
}
