// Package service holds the support bot's use cases: what happens when a
// shopper opens a chat, types, or taps a button.
package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/ports"
)

// Inbound is one thing a shopper did, already stripped of Bot API shape.
type Inbound struct {
	ChatID          string
	ChatType        string
	TelegramUserID  *int64
	Username        string
	Text            string
	CallbackQueryID string
	CallbackData    string
}

// IsCallback reports whether this was a button tap rather than typed text.
func (in Inbound) IsCallback() bool { return in.CallbackQueryID != "" }

// IDGenerator mints conversation ids. Injected so tests read deterministically.
type IDGenerator func() string

// Clock is the service's view of time, injected for the same reason.
type Clock func() time.Time

// Router answers inbound messages.
//
// Phase 2 scope (docs/support-telegram-bot.md): open the root menu and record
// the conversation. Walking into a topic is Phase 3 — a tap is acknowledged so
// the button stops spinning, and nothing else happens yet.
type Router struct {
	conversations ports.ConversationRepository
	bot           ports.Bot
	newID         IDGenerator
	now           Clock
}

func NewRouter(conversations ports.ConversationRepository, bot ports.Bot, newID IDGenerator, now Clock) *Router {
	if newID == nil {
		newID = func() string { return "" }
	}
	if now == nil {
		now = time.Now
	}
	return &Router{conversations: conversations, bot: bot, newID: newID, now: now}
}

// Handle processes one inbound event.
func (r *Router) Handle(ctx context.Context, in Inbound) error {
	if r == nil {
		return nil
	}
	// Private chats only. A bot added to a group or channel would otherwise
	// answer every passing message there, and a customer consultation has no
	// business happening in front of an audience.
	if in.ChatType != "" && in.ChatType != "private" {
		return nil
	}
	if strings.TrimSpace(in.ChatID) == "" {
		return fmt.Errorf("inbound chat id is required")
	}

	conversation, err := r.ensureConversation(ctx, in)
	if err != nil {
		return err
	}

	if in.IsCallback() {
		// Telegram spins the button until this lands, so it is the first thing
		// done and it happens whatever the tap turns out to mean.
		if err := r.bot.AnswerCallback(ctx, in.CallbackQueryID, ""); err != nil {
			return fmt.Errorf("answer callback: %w", err)
		}
		// Routing into a topic arrives in Phase 3. Until then a tap leaves the
		// menu where it is rather than pretending to answer.
		return nil
	}

	conversation.Node = domain.NodeRoot
	if err := r.conversations.Save(ctx, conversation); err != nil {
		return fmt.Errorf("save conversation: %w", err)
	}
	return r.sendRootMenu(ctx, in.ChatID)
}

func (r *Router) sendRootMenu(ctx context.Context, chatID string) error {
	buttons := make([]ports.MenuButton, 0, len(domain.RootMenu))
	for _, item := range domain.RootMenu {
		buttons = append(buttons, ports.MenuButton{
			Label:        item.Label,
			CallbackData: domain.CallbackData(item.Node),
		})
	}
	if err := r.bot.ReplyMenu(ctx, chatID, domain.RootGreeting, buttons); err != nil {
		return fmt.Errorf("send root menu: %w", err)
	}
	return nil
}

// ensureConversation loads the chat's row or starts one.
func (r *Router) ensureConversation(ctx context.Context, in Inbound) (*domain.Conversation, error) {
	existing, err := r.conversations.FindByChatID(ctx, in.ChatID)
	if err != nil {
		return nil, fmt.Errorf("find conversation: %w", err)
	}

	now := r.now()
	if existing == nil {
		existing = domain.NewConversation(r.newID(), in.ChatID, now)
		// Only a first /start carries the deep-link payload; a later one would
		// overwrite the context the shopper actually arrived with.
		existing.EntryPayload = startPayload(in.Text)
	}
	existing.Touch(now)
	if in.TelegramUserID != nil {
		existing.TelegramUserID = in.TelegramUserID
	}
	if name := strings.TrimSpace(in.Username); name != "" {
		existing.Username = name
	}
	return existing, nil
}

// startPayload returns the deep-link payload of a "/start <payload>" message.
//
// The storefront button will encode where the shopper came from into it
// (Phase 6). It is stored as sent and never trusted: it is a hint about which
// page they were on, never an identity and never a permission.
func startPayload(text string) string {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) < 2 {
		return ""
	}
	command := fields[0]
	if at := strings.Index(command, "@"); at >= 0 {
		command = command[:at]
	}
	if command != "/start" {
		return ""
	}
	if len(fields[1]) > 64 {
		// Telegram caps a start payload at 64 characters; anything longer did
		// not come from a deep link this app built.
		return ""
	}
	return fields[1]
}
