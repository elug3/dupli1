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

// excerptRunes bounds what an ops alert quotes of a shopper's message.
const excerptRunes = 180

// Router answers inbound messages by walking the consultation menu, and hands
// a conversation to staff when the shopper asks for one.
type Router struct {
	conversations ports.ConversationRepository
	answers       ports.AnswerRepository
	inquiries     ports.InquiryRepository
	messages      ports.MessageRepository
	publisher     ports.InquiryPublisher
	bot           ports.Bot
	hours         domain.BusinessHours
	newID         IDGenerator
	now           Clock
}

// Deps are the Router's collaborators. A struct rather than a parameter list
// because the list had grown past the point where call sites read clearly.
type Deps struct {
	Conversations ports.ConversationRepository
	Answers       ports.AnswerRepository
	Inquiries     ports.InquiryRepository
	Messages      ports.MessageRepository
	Publisher     ports.InquiryPublisher
	Bot           ports.Bot
	Hours         domain.BusinessHours
	NewID         IDGenerator
	Now           Clock
}

func NewRouter(deps Deps) *Router {
	newID := deps.NewID
	if newID == nil {
		newID = func() string { return "" }
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	hours := deps.Hours
	if hours.Location == nil {
		hours = domain.DefaultBusinessHours()
	}
	return &Router{
		conversations: deps.Conversations,
		answers:       deps.Answers,
		inquiries:     deps.Inquiries,
		messages:      deps.Messages,
		publisher:     deps.Publisher,
		bot:           deps.Bot,
		hours:         hours,
		newID:         newID,
		now:           now,
	}
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
	// Save before anything references the conversation. Messages carry a
	// foreign key to it, so recording what a shopper typed on their very first
	// message would otherwise fail against a real database — and take the
	// whole update, menu included, down with it.
	if err := r.conversations.Save(ctx, conversation); err != nil {
		return fmt.Errorf("save conversation: %w", err)
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
	if err := r.recordInbound(ctx, in, conversation); err != nil {
		return err
	}

	// While staff have an open inquiry with this chat, typed text is part of
	// that conversation, not a request to start over. Re-opening the menu here
	// would talk over the shopper mid-sentence.
	open, err := r.openInquiry(ctx, in.ChatID)
	if err != nil {
		return err
	}
	if open != nil {
		return nil
	}

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

// recordInbound stores a shopper's message against the conversation, and
// against the open inquiry when there is one, so staff read the whole thread.
func (r *Router) recordInbound(ctx context.Context, in Inbound, conversation *domain.Conversation) error {
	if r.messages == nil || strings.TrimSpace(in.Text) == "" {
		return nil
	}
	open, err := r.openInquiry(ctx, in.ChatID)
	if err != nil {
		return err
	}
	inquiryID := ""
	if open != nil {
		inquiryID = open.ID
	}
	message := &domain.Message{
		ID:             r.newID(),
		ConversationID: conversation.ID,
		InquiryID:      inquiryID,
		Direction:      domain.DirectionInbound,
		Body:           in.Text,
		CreatedAt:      r.now(),
	}
	if err := r.messages.Append(ctx, message); err != nil {
		return fmt.Errorf("record message: %w", err)
	}
	return nil
}

func (r *Router) openInquiry(ctx context.Context, chatID string) (*domain.Inquiry, error) {
	if r.inquiries == nil {
		return nil, nil
	}
	open, err := r.inquiries.FindOpenByChatID(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("find open inquiry: %w", err)
	}
	return open, nil
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
	if domain.Nodes[node].Escalates {
		note, err := r.escalate(ctx, conversation, node)
		if err != nil {
			return err
		}
		text += "\n\n" + note
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

// escalate opens an inquiry for the conversation and announces it, returning
// the line the shopper is told.
//
// Re-tapping "상담원 연결" while an inquiry is already open must not queue a
// second one: staff would see two rows for one shopper and answer twice.
func (r *Router) escalate(ctx context.Context, conversation *domain.Conversation, topic string) (string, error) {
	if r.inquiries == nil {
		return r.waitNote(), nil
	}

	open, err := r.openInquiry(ctx, conversation.ChatID)
	if err != nil {
		return "", err
	}
	if open != nil {
		return r.waitNote(), nil
	}

	now := r.now()
	inquiry := domain.NewInquiry(r.newID(), conversation.ID, conversation.ChatID, topic, now)
	if err := r.inquiries.Save(ctx, inquiry); err != nil {
		return "", fmt.Errorf("save inquiry: %w", err)
	}

	if r.publisher != nil {
		err := r.publisher.InquiryOpened(ctx, ports.InquiryOpened{
			InquiryID:    inquiry.ID,
			ChatID:       conversation.ChatID,
			Topic:        topic,
			Language:     conversation.Language,
			Username:     conversation.Username,
			EntryContext: conversation.EntryPayload,
			Excerpt:      r.lastExcerpt(ctx, conversation),
			AfterHours:   !r.hours.IsOpen(now),
		})
		if err != nil {
			// The inquiry is saved and the shopper has been told someone will
			// answer. Losing the alert is bad, but unsaying that is worse, so
			// the error travels up to be logged rather than shown.
			return "", fmt.Errorf("publish inquiry opened: %w", err)
		}
	}
	return r.waitNote(), nil
}

// waitNote tells the shopper what happens next.
//
// It names the service window and never a day: without a holiday calendar,
// "내일" said on the eve of Chuseok is wrong by four days. See
// docs/support-telegram-bot.md.
func (r *Router) waitNote() string {
	if r.hours.PromisesSameDay(r.now()) {
		return "✅ 문의가 접수되었습니다. 상담원이 순서대로 답변드리겠습니다."
	}
	return "✅ 문의가 접수되었습니다.\n지금은 상담 시간이 아닙니다 — 상담 시간(<b>" +
		r.hours.Window() + "</b>, 공휴일 휴무)에 순서대로 답변드립니다."
}

// lastExcerpt quotes what the shopper said, for the ops alert.
func (r *Router) lastExcerpt(ctx context.Context, conversation *domain.Conversation) string {
	if r.messages == nil {
		return ""
	}
	reader, ok := r.messages.(ports.MessageReader)
	if !ok {
		return ""
	}
	body, err := reader.LastInbound(ctx, conversation.ID)
	if err != nil || strings.TrimSpace(body) == "" {
		return ""
	}
	return domain.Excerpt(body, excerptRunes)
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
