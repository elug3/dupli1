package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/elug3/dupli1/notification/pkg/domain"
)

// SubscriptionLookup resolves stored Telegram subscriptions for inbound messages.
type SubscriptionLookup interface {
	RegisterFromMessage(ctx context.Context, in SubscriptionInput) (*domain.TelegramSubscription, error)
	FindForMessage(ctx context.Context, chatID string, userID *int64) (*domain.TelegramSubscription, error)
	UpdateMetadata(ctx context.Context, id string, in SubscriptionInput) error
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
		p.refreshMetadata(ctx, existing, msg, userID)
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

// refreshMetadata keeps the stored display fields in step with the chat.
//
// They are captured once, when a chat registers, so a group renamed afterwards
// kept its old label in the manager UI forever. /start is the only moment fresh
// metadata arrives, and the write only happens when something actually changed.
//
// A failure here is logged rather than returned: the reply is the part the
// sender is waiting on, and a stale label is not worth losing it over.
func (p *UpdateProcessor) refreshMetadata(ctx context.Context, sub *domain.TelegramSubscription, msg *Message, userID *int64) {
	if p.Lookup == nil || sub == nil {
		return
	}
	in := p.subscriptionInput(msg, userID)
	if !metadataChanged(sub, in) {
		return
	}
	if err := p.Lookup.UpdateMetadata(ctx, sub.ID, in); err != nil {
		log.Printf("telegram refresh metadata for chat %s: %v", sub.ChatID, err)
	}
}

// metadataChanged reports whether in carries a value that differs from what is
// stored. An empty inbound field is absent, not a deletion.
func metadataChanged(sub *domain.TelegramSubscription, in SubscriptionInput) bool {
	for _, field := range []struct{ stored, incoming string }{
		{sub.ChatType, in.ChatType},
		{sub.ChatLabel, in.ChatLabel},
		{sub.Username, in.Username},
	} {
		incoming := strings.TrimSpace(field.incoming)
		if incoming != "" && incoming != field.stored {
			return true
		}
	}
	return false
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
