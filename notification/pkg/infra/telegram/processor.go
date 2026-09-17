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
	Client *Client
	Lookup SubscriptionLookup
	Policy AccessPolicy
}

// Handle registers and answers an inbound Telegram message.
//
// Only an explicit /start registers a chat. Registering on every inbound
// message meant a bot added to a group months ago, or a passing "hi" from a
// stranger, left a pending row no manager recognises. The pending
// acknowledgement is sent once, when the row is created — repeating /start
// while a manager has not acted yet is silent.
func (p *UpdateProcessor) Handle(ctx context.Context, update Update) error {
	if p == nil || update.Message == nil {
		return nil
	}
	msg := update.Message
	if !IsStartCommand(msg.Text) {
		return nil
	}

	var userID *int64
	if msg.From != nil {
		id := msg.From.ID
		userID = &id
	}

	existing, err := p.findExisting(ctx, msg.Chat.FormatID(), userID)
	if err != nil {
		return fmt.Errorf("look up telegram subscription: %w", err)
	}
	if existing != nil {
		// Known chat: welcome it once accepted, stay quiet while it is pending
		// or rejected.
		if existing.IsAccepted() {
			return p.reply(ctx, msg, FormatStartReply(msg.Chat))
		}
		return nil
	}

	// Allowed by the transitional env allowlist — nothing to register.
	if p.Policy != nil && p.Policy.AllowsIncoming(msg.Chat, msg.From) {
		return p.reply(ctx, msg, FormatStartReply(msg.Chat))
	}

	if p.Lookup == nil {
		// Nowhere to register, and no allowlist match: an unknown sender
		// learns nothing about this bot.
		return nil
	}

	sub, err := p.Lookup.RegisterFromMessage(ctx, p.subscriptionInput(msg, userID))
	if err != nil {
		return fmt.Errorf("register telegram subscription: %w", err)
	}
	if sub != nil && sub.IsAccepted() {
		// Raced with a manager accepting this chat.
		return p.reply(ctx, msg, FormatStartReply(msg.Chat))
	}
	return p.reply(ctx, msg, FormatPendingReply(msg.Chat))
}

func (p *UpdateProcessor) subscriptionInput(msg *Message, userID *int64) SubscriptionInput {
	in := SubscriptionInput{
		ChatID:    msg.Chat.FormatID(),
		ChatType:  msg.Chat.Type,
		ChatLabel: chatLabelText(msg.Chat),
	}
	if msg.From != nil {
		in.TelegramUserID = userID
		in.Username = msg.From.Username
	}
	return in
}

func (p *UpdateProcessor) findExisting(ctx context.Context, chatID string, userID *int64) (*domain.TelegramSubscription, error) {
	if p.Lookup == nil {
		return nil, nil
	}
	return p.Lookup.FindForMessage(ctx, chatID, userID)
}

// reply answers a command with Reply rather than Send: a pending chat is not
// outbound-allowlisted yet, so the ack has to bypass AllowsChat.
func (p *UpdateProcessor) reply(ctx context.Context, msg *Message, text string) error {
	if p.Client == nil {
		return nil
	}
	return p.Client.Reply(ctx, msg.Chat.FormatID(), text)
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
