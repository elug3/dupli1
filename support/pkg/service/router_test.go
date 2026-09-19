package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/infra/memory"
	"github.com/elug3/dupli1/support/pkg/ports"
	"github.com/elug3/dupli1/support/pkg/service"
)

type sentMenu struct {
	chatID  string
	text    string
	buttons []ports.MenuButton
}

type editedMenu struct {
	chatID    string
	messageID int64
	text      string
	buttons   []ports.MenuButton
}

type sentReply struct {
	chatID string
	text   string
}

type fakeBot struct {
	menus     []sentMenu
	edits     []editedMenu
	replies   []sentReply
	callbacks []string
	replyErr  error
}

func (b *fakeBot) Reply(_ context.Context, chatID string, text string) error {
	if b.replyErr != nil {
		return b.replyErr
	}
	b.replies = append(b.replies, sentReply{chatID: chatID, text: text})
	return nil
}

func (b *fakeBot) ReplyMenu(_ context.Context, chatID string, text string, buttons []ports.MenuButton) error {
	b.menus = append(b.menus, sentMenu{chatID: chatID, text: text, buttons: buttons})
	return nil
}

func (b *fakeBot) EditMenu(_ context.Context, chatID string, messageID int64, text string, buttons []ports.MenuButton) error {
	b.edits = append(b.edits, editedMenu{chatID: chatID, messageID: messageID, text: text, buttons: buttons})
	return nil
}

func (b *fakeBot) AnswerCallback(_ context.Context, callbackQueryID string, _ string) error {
	b.callbacks = append(b.callbacks, callbackQueryID)
	return nil
}

// openHours is a Thursday at 11:00 KST — inside the service window.
var openHours = time.Date(2026, 9, 17, 11, 0, 0, 0, seoul())

func seoul() *time.Location {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		return time.FixedZone("KST", 9*60*60)
	}
	return loc
}

type harness struct {
	router        *service.Router
	bot           *fakeBot
	conversations *memory.ConversationRepository
	inquiries     *memory.InquiryRepository
	messages      *memory.MessageRepository
	published     *fakePublisher
}

type fakePublisher struct {
	opened []ports.InquiryOpened
	err    error
}

func (p *fakePublisher) InquiryOpened(_ context.Context, in ports.InquiryOpened) error {
	if p.err != nil {
		return p.err
	}
	p.opened = append(p.opened, in)
	return nil
}

func newHarnessAt(now time.Time) *harness {
	h := &harness{
		bot:           &fakeBot{},
		conversations: memory.NewConversationRepository(),
		inquiries:     memory.NewInquiryRepository(),
		messages:      memory.NewMessageRepository(),
		published:     &fakePublisher{},
	}
	n := 0
	h.router = service.NewRouter(service.Deps{
		Conversations: h.conversations,
		Answers:       memory.NewAnswerRepository(),
		Inquiries:     h.inquiries,
		Messages:      h.messages,
		Publisher:     h.published,
		Bot:           h.bot,
		NewID: func() string {
			n++
			return "id-" + string(rune('0'+n))
		},
		Now: func() time.Time { return now },
	})
	return h
}

func newRouter() (*service.Router, *fakeBot, *memory.ConversationRepository) {
	h := newHarnessAt(openHours)
	return h.router, h.bot, h.conversations
}

func TestStartOpensTheRootMenu(t *testing.T) {
	router, bot, repo := newRouter()

	err := router.Handle(t.Context(), service.Inbound{ChatID: "42", ChatType: "private", Text: "/start"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if len(bot.menus) != 1 {
		t.Fatalf("menus sent = %d, want 1", len(bot.menus))
	}
	menu := bot.menus[0]
	if menu.chatID != "42" {
		t.Fatalf("chat id = %q", menu.chatID)
	}
	if len(menu.buttons) != len(domain.RootMenu()) {
		t.Fatalf("buttons = %d, want %d", len(menu.buttons), len(domain.RootMenu()))
	}
	if menu.buttons[0].CallbackData != domain.CallbackData(domain.NodeOrder) {
		t.Fatalf("first button routes to %q", menu.buttons[0].CallbackData)
	}

	saved, _ := repo.FindByChatID(t.Context(), "42")
	if saved == nil || saved.Node != domain.NodeRoot {
		t.Fatalf("conversation = %+v, want one parked at the root", saved)
	}
	if saved.Language != domain.DefaultLanguage {
		t.Fatalf("language = %q, want the launch default", saved.Language)
	}
}

func TestAnyTypedMessageOpensTheMenu(t *testing.T) {
	// A shopper who types "안녕하세요" must not be met with silence just
	// because they did not send /start.
	router, bot, _ := newRouter()

	if err := router.Handle(t.Context(), service.Inbound{ChatID: "42", ChatType: "private", Text: "안녕하세요"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(bot.menus) != 1 {
		t.Fatalf("menus sent = %d, want the menu", len(bot.menus))
	}
}

func TestStartCapturesTheDeepLinkPayload(t *testing.T) {
	router, _, repo := newRouter()

	if err := router.Handle(t.Context(), service.Inbound{ChatID: "42", ChatType: "private", Text: "/start c-louis-vuitton-ko"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	saved, _ := repo.FindByChatID(t.Context(), "42")
	if saved.EntryPayload != "c-louis-vuitton-ko" {
		t.Fatalf("entry payload = %q", saved.EntryPayload)
	}
}

func TestEntryPayloadIsKeptFromTheFirstStartOnly(t *testing.T) {
	router, _, repo := newRouter()
	ctx := t.Context()

	_ = router.Handle(ctx, service.Inbound{ChatID: "42", ChatType: "private", Text: "/start p-01HXYZ-ko"})
	_ = router.Handle(ctx, service.Inbound{ChatID: "42", ChatType: "private", Text: "/start h-ko"})

	saved, _ := repo.FindByChatID(ctx, "42")
	if saved.EntryPayload != "p-01HXYZ-ko" {
		t.Fatalf("entry payload = %q, want the context they first arrived with", saved.EntryPayload)
	}
}

func TestOverlongStartPayloadIsIgnored(t *testing.T) {
	router, _, repo := newRouter()
	long := make([]byte, 65)
	for i := range long {
		long[i] = 'a'
	}

	if err := router.Handle(t.Context(), service.Inbound{ChatID: "42", ChatType: "private", Text: "/start " + string(long)}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	saved, _ := repo.FindByChatID(t.Context(), "42")
	if saved.EntryPayload != "" {
		t.Fatalf("entry payload = %q, want none: Telegram caps a start payload at 64 chars", saved.EntryPayload)
	}
}

func TestGroupAndChannelChatsAreIgnored(t *testing.T) {
	// A consultation has no business happening in front of an audience, and a
	// bot added to a group must not answer every passing message.
	for _, chatType := range []string{"group", "supergroup", "channel"} {
		router, bot, repo := newRouter()
		if err := router.Handle(t.Context(), service.Inbound{ChatID: "-100", ChatType: chatType, Text: "/start"}); err != nil {
			t.Fatalf("Handle(%s): %v", chatType, err)
		}
		if len(bot.menus) != 0 {
			t.Fatalf("%s chat got a menu", chatType)
		}
		if saved, _ := repo.FindByChatID(t.Context(), "-100"); saved != nil {
			t.Fatalf("%s chat was stored", chatType)
		}
	}
}

func TestButtonTapIsAlwaysAcknowledged(t *testing.T) {
	// Telegram spins the button until answerCallbackQuery lands.
	router, bot, _ := newRouter()

	err := router.Handle(t.Context(), service.Inbound{
		ChatID: "42", ChatType: "private", MessageID: 555,
		CallbackQueryID: "cbq-1", CallbackData: domain.CallbackData(domain.NodeOrder),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(bot.callbacks) != 1 || bot.callbacks[0] != "cbq-1" {
		t.Fatalf("callbacks = %v, want the tap acknowledged", bot.callbacks)
	}
}

func TestTapWalksTheMenuInPlace(t *testing.T) {
	router, bot, repo := newRouter()

	err := router.Handle(t.Context(), service.Inbound{
		ChatID: "42", ChatType: "private", MessageID: 555,
		CallbackQueryID: "cbq-1", CallbackData: domain.CallbackData(domain.NodeReturn),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if len(bot.menus) != 0 {
		t.Fatalf("a tap must edit the open menu, not stack a new message (%d sent)", len(bot.menus))
	}
	if len(bot.edits) != 1 {
		t.Fatalf("edits = %d, want 1", len(bot.edits))
	}
	edit := bot.edits[0]
	if edit.messageID != 555 {
		t.Fatalf("edited message id = %d, want the tapped message", edit.messageID)
	}
	if edit.text != domain.AnswerFor(domain.NodeReturn) {
		t.Fatalf("edited text = %q, want the node's copy", edit.text)
	}
	// Every non-root node offers a way back, or a shopper is stranded.
	last := edit.buttons[len(edit.buttons)-1]
	if last.Label != domain.BackLabel || last.CallbackData != domain.CallbackData(domain.NodeRoot) {
		t.Fatalf("last button = %+v, want a route back to the root", last)
	}

	saved, _ := repo.FindByChatID(t.Context(), "42")
	if saved.Node != domain.NodeReturn {
		t.Fatalf("conversation node = %q, want the tapped node", saved.Node)
	}
}

func TestEveryNodeRendersWhenTapped(t *testing.T) {
	// "Every node reachable" is the phase's acceptance criterion, so walk them.
	for id := range domain.Nodes {
		router, bot, _ := newRouter()
		err := router.Handle(t.Context(), service.Inbound{
			ChatID: "42", ChatType: "private", MessageID: 555,
			CallbackQueryID: "cbq", CallbackData: domain.CallbackData(id),
		})
		if err != nil {
			t.Fatalf("Handle(%s): %v", id, err)
		}
		if len(bot.edits) != 1 {
			t.Fatalf("node %q produced %d edits, want 1", id, len(bot.edits))
		}
		if strings.TrimSpace(bot.edits[0].text) == "" {
			t.Fatalf("node %q rendered an empty message, which Telegram rejects", id)
		}
	}
}

func TestStaleTapReopensTheRootMenu(t *testing.T) {
	// A button from a menu version that no longer exists is still tappable.
	router, bot, repo := newRouter()

	err := router.Handle(t.Context(), service.Inbound{
		ChatID: "42", ChatType: "private", MessageID: 555,
		CallbackQueryID: "cbq-1", CallbackData: "v0:gone",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if bot.edits[0].text != domain.RootGreeting {
		t.Fatalf("stale tap rendered %q, want the root menu", bot.edits[0].text)
	}
	saved, _ := repo.FindByChatID(t.Context(), "42")
	if saved.Node != domain.NodeRoot {
		t.Fatalf("conversation node = %q, want root", saved.Node)
	}
}

func TestTapWithNoMessageSendsAFreshMenu(t *testing.T) {
	// Telegram omits the message for taps on very old ones; the answer must
	// still arrive rather than being dropped for want of something to edit.
	router, bot, _ := newRouter()

	err := router.Handle(t.Context(), service.Inbound{
		ChatID: "42", ChatType: "private",
		CallbackQueryID: "cbq-1", CallbackData: domain.CallbackData(domain.NodePayment),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(bot.edits) != 0 {
		t.Fatal("there was no message to edit")
	}
	if len(bot.menus) != 1 || bot.menus[0].text != domain.AnswerFor(domain.NodePayment) {
		t.Fatalf("menus = %+v, want the node sent fresh", bot.menus)
	}
}

func TestStoredCopyBeatsTheSeededDefault(t *testing.T) {
	// Staff edit copy from the inbox; the bot must serve their words, not the
	// text a deploy shipped.
	repo := memory.NewConversationRepository()
	bot := &fakeBot{}
	router := service.NewRouter(service.Deps{
		Conversations: repo,
		Answers:       &stubAnswers{body: "<b>직접 수정한 안내</b>"},
		Bot:           bot,
		NewID:         func() string { return "conv-1" },
		Now:           func() time.Time { return openHours },
	})

	err := router.Handle(t.Context(), service.Inbound{
		ChatID: "42", ChatType: "private", MessageID: 5,
		CallbackQueryID: "cbq-1", CallbackData: domain.CallbackData(domain.NodeReturn),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if bot.edits[0].text != "<b>직접 수정한 안내</b>" {
		t.Fatalf("text = %q, want the stored copy", bot.edits[0].text)
	}
}

func TestMissingCopyFallsBackToTheSeededText(t *testing.T) {
	// A deleted row must not render an empty message: Telegram rejects one, so
	// a single missing row would otherwise kill the node.
	repo := memory.NewConversationRepository()
	bot := &fakeBot{}
	router := service.NewRouter(service.Deps{
		Conversations: repo,
		Answers:       &stubAnswers{body: "  "},
		Bot:           bot,
		NewID:         func() string { return "conv-1" },
		Now:           func() time.Time { return openHours },
	})

	err := router.Handle(t.Context(), service.Inbound{
		ChatID: "42", ChatType: "private", MessageID: 5,
		CallbackQueryID: "cbq-1", CallbackData: domain.CallbackData(domain.NodeReturn),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if bot.edits[0].text != domain.AnswerFor(domain.NodeReturn) {
		t.Fatalf("text = %q, want the seeded fallback", bot.edits[0].text)
	}
}

type stubAnswers struct{ body string }

func (s *stubAnswers) Body(context.Context, string, string) (string, error) { return s.body, nil }

func (s *stubAnswers) All(context.Context, string) (map[string]string, error) { return nil, nil }

func (s *stubAnswers) Put(context.Context, string, string, string, string) error { return nil }

func TestRepeatVisitorKeepsOneConversation(t *testing.T) {
	router, _, repo := newRouter()
	ctx := t.Context()
	userID := int64(99)

	_ = router.Handle(ctx, service.Inbound{ChatID: "42", ChatType: "private", Text: "/start"})
	_ = router.Handle(ctx, service.Inbound{ChatID: "42", ChatType: "private", Text: "hello", TelegramUserID: &userID, Username: "shopper"})

	saved, _ := repo.FindByChatID(ctx, "42")
	if saved.ID != "id-1" {
		t.Fatalf("conversation id = %q, want the original row reused", saved.ID)
	}
	if saved.TelegramUserID == nil || *saved.TelegramUserID != 99 || saved.Username != "shopper" {
		t.Fatalf("identity not refreshed: %+v", saved)
	}
}

func TestChatIDIsRequired(t *testing.T) {
	router, _, _ := newRouter()
	if err := router.Handle(t.Context(), service.Inbound{ChatType: "private", Text: "/start"}); err == nil {
		t.Fatal("expected an error for a missing chat id")
	}
}
