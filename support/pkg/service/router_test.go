package service_test

import (
	"context"
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

type fakeBot struct {
	menus     []sentMenu
	callbacks []string
}

func (b *fakeBot) ReplyMenu(_ context.Context, chatID string, text string, buttons []ports.MenuButton) error {
	b.menus = append(b.menus, sentMenu{chatID: chatID, text: text, buttons: buttons})
	return nil
}

func (b *fakeBot) AnswerCallback(_ context.Context, callbackQueryID string, _ string) error {
	b.callbacks = append(b.callbacks, callbackQueryID)
	return nil
}

func newRouter() (*service.Router, *fakeBot, *memory.ConversationRepository) {
	repo := memory.NewConversationRepository()
	bot := &fakeBot{}
	fixed := time.Date(2026, 9, 19, 11, 0, 0, 0, time.UTC)
	n := 0
	router := service.NewRouter(repo, bot, func() string {
		n++
		return "conv-" + string(rune('0'+n))
	}, func() time.Time { return fixed })
	return router, bot, repo
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
	if len(menu.buttons) != len(domain.RootMenu) {
		t.Fatalf("buttons = %d, want %d", len(menu.buttons), len(domain.RootMenu))
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
	// Telegram spins the button until answerCallbackQuery lands. Routing into a
	// topic is Phase 3, but the spinner must stop today.
	router, bot, _ := newRouter()

	err := router.Handle(t.Context(), service.Inbound{
		ChatID: "42", ChatType: "private",
		CallbackQueryID: "cbq-1", CallbackData: domain.CallbackData(domain.NodeOrder),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(bot.callbacks) != 1 || bot.callbacks[0] != "cbq-1" {
		t.Fatalf("callbacks = %v, want the tap acknowledged", bot.callbacks)
	}
}

func TestRepeatVisitorKeepsOneConversation(t *testing.T) {
	router, _, repo := newRouter()
	ctx := t.Context()
	userID := int64(99)

	_ = router.Handle(ctx, service.Inbound{ChatID: "42", ChatType: "private", Text: "/start"})
	_ = router.Handle(ctx, service.Inbound{ChatID: "42", ChatType: "private", Text: "hello", TelegramUserID: &userID, Username: "shopper"})

	saved, _ := repo.FindByChatID(ctx, "42")
	if saved.ID != "conv-1" {
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
