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
	MessageID       int64
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

// Router answers inbound messages by walking the consultation menu.
type Router struct {
	conversations ports.ConversationRepository
	answers       ports.AnswerRepository
	bot           ports.Bot
	newID         IDGenerator
	now           Clock
}

func NewRouter(
	conversations ports.ConversationRepository,
	answers ports.AnswerRepository,
	bot ports.Bot,
	newID IDGenerator,
	now Clock,
) *Router {
	if newID == nil {
		newID = func() string { return "" }
	}
	if now == nil {
		now = time.Now
	}
	return &Router{conversations: conversations, answers: answers, bot: bot, newID: newID, now: now}
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
		return r.handleTap(ctx, in, conversation)
	}
	return r.handleMessage(ctx, in, conversation)
}

// handleMessage answers typed text by opening the root menu.
//
// Any text does, not only /start: a shopper who writes "안녕하세요" must not be
// met with silence for skipping the command. Free text is never trapped by the
// menu — the shopper's position stays where the buttons put it.
func (r *Router) handleMessage(ctx context.Context, in Inbound, conversation *domain.Conversation) error {
	conversation.Node = domain.NodeRoot
	if err := r.conversations.Save(ctx, conversation); err != nil {
		return fmt.Errorf("save conversation: %w", err)
	}

	text, buttons, err := r.render(ctx, domain.NodeRoot, conversation.Language)
	if err != nil {
		return err
	}
	if err := r.bot.ReplyMenu(ctx, in.ChatID, text, buttons); err != nil {
		return fmt.Errorf("send root menu: %w", err)
	}
	return nil
}

// handleTap walks into the tapped node, replacing the menu in place.
func (r *Router) handleTap(ctx context.Context, in Inbound, conversation *domain.Conversation) error {
	// Telegram spins the button until this lands, so it happens first and it
	// happens whatever the tap turns out to mean.
	if err := r.bot.AnswerCallback(ctx, in.CallbackQueryID, ""); err != nil {
		return fmt.Errorf("answer callback: %w", err)
	}

	node := domain.ParseCallbackData(in.CallbackData)
	conversation.Node = node
	if err := r.conversations.Save(ctx, conversation); err != nil {
		return fmt.Errorf("save conversation: %w", err)
	}

	text, buttons, err := r.render(ctx, node, conversation.Language)
	if err != nil {
		return err
	}

	// No message to edit (Telegram omits it for taps on very old messages), so
	// send a fresh one rather than dropping the answer.
	if in.MessageID == 0 {
		if err := r.bot.ReplyMenu(ctx, in.ChatID, text, buttons); err != nil {
			return fmt.Errorf("send menu: %w", err)
		}
		return nil
	}
	if err := r.bot.EditMenu(ctx, in.ChatID, in.MessageID, text, buttons); err != nil {
		return fmt.Errorf("edit menu: %w", err)
	}
	return nil
}

// render builds a node's message: its stored copy, its child buttons, and a way
// back to the root from anywhere but the root itself.
func (r *Router) render(ctx context.Context, node, language string) (string, []ports.MenuButton, error) {
	text, err := r.answerFor(ctx, node, language)
	if err != nil {
		return "", nil, err
	}

	children := domain.ChildrenOf(node)
	buttons := make([]ports.MenuButton, 0, len(children)+1)
	for _, child := range children {
		buttons = append(buttons, ports.MenuButton{
			Label:        child.Label,
			CallbackData: domain.CallbackData(child.ID),
		})
	}
	if node != domain.NodeRoot {
		buttons = append(buttons, ports.MenuButton{
			Label:        domain.BackLabel,
			CallbackData: domain.CallbackData(domain.NodeRoot),
		})
	}
	return text, buttons, nil
}

// answerFor reads a node's copy, falling back to the seeded text.
//
// The fallback matters on the day someone deletes a row: a node with no copy
// would otherwise render as an empty message, which Telegram rejects outright,
// turning one missing row into a dead menu.
func (r *Router) answerFor(ctx context.Context, node, language string) (string, error) {
	if r.answers != nil {
		body, err := r.answers.Body(ctx, node, language)
		if err != nil {
			return "", fmt.Errorf("load answer for %q: %w", node, err)
		}
		if strings.TrimSpace(body) != "" {
			return body, nil
		}
	}
	if seeded := domain.AnswerFor(node); seeded != "" {
		return seeded, nil
	}
	return domain.RootGreeting, nil
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
