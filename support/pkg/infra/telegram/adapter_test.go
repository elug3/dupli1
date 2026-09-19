package telegram_test

import (
	"context"
	"testing"

	tg "github.com/elug3/dupli1/shared/pkg/telegram"
	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/infra/memory"
	telegraminfra "github.com/elug3/dupli1/support/pkg/infra/telegram"
	"github.com/elug3/dupli1/support/pkg/ports"
	"github.com/elug3/dupli1/support/pkg/service"
)

type capturingBot struct {
	menus     int
	edits     int
	callbacks []string
}

func (b *capturingBot) ReplyMenu(context.Context, string, string, []ports.MenuButton) error {
	b.menus++
	return nil
}

func (b *capturingBot) EditMenu(context.Context, string, int64, string, []ports.MenuButton) error {
	b.edits++
	return nil
}

func (b *capturingBot) AnswerCallback(_ context.Context, id string, _ string) error {
	b.callbacks = append(b.callbacks, id)
	return nil
}

func newProcessor() (*telegraminfra.UpdateProcessor, *capturingBot, *memory.ConversationRepository) {
	repo := memory.NewConversationRepository()
	bot := &capturingBot{}
	router := service.NewRouter(service.Deps{
		Conversations: repo,
		Answers:       memory.NewAnswerRepository(),
		Bot:           bot,
		NewID:         func() string { return "conv-1" },
	})
	return &telegraminfra.UpdateProcessor{Router: router}, bot, repo
}

func TestProcessorRoutesAMessage(t *testing.T) {
	processor, bot, repo := newProcessor()
	userID := int64(99)

	update := tg.Update{
		UpdateID: 1,
		Message: &tg.Message{
			MessageID: 2,
			Text:      "/start h-ko",
			Chat:      tg.Chat{ID: 42, Type: "private"},
			From:      &tg.User{ID: userID, Username: "shopper"},
		},
	}
	if err := processor.Handle(t.Context(), update); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if bot.menus != 1 {
		t.Fatalf("menus = %d, want the root menu", bot.menus)
	}
	saved, _ := repo.FindByChatID(t.Context(), "42")
	if saved == nil || saved.Username != "shopper" || saved.EntryPayload != "h-ko" {
		t.Fatalf("conversation = %+v", saved)
	}
}

func TestProcessorRoutesACallbackQuery(t *testing.T) {
	processor, bot, _ := newProcessor()

	update := tg.Update{
		UpdateID: 2,
		CallbackQuery: &tg.CallbackQuery{
			ID:      "cbq-1",
			From:    &tg.User{ID: 99},
			Message: &tg.Message{MessageID: 555, Chat: tg.Chat{ID: 42, Type: "private"}},
			Data:    domain.CallbackData(domain.NodeReturn),
		},
	}
	if err := processor.Handle(t.Context(), update); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(bot.callbacks) != 1 || bot.callbacks[0] != "cbq-1" {
		t.Fatalf("callbacks = %v", bot.callbacks)
	}
	// The message id from the callback is what the menu is edited in place by.
	if bot.edits != 1 {
		t.Fatalf("edits = %d, want the menu walked in place", bot.edits)
	}
}

func TestProcessorIgnoresUpdatesItDoesNotServe(t *testing.T) {
	// The webhook asks only for messages and callback queries, but polling
	// delivers every type, so an unknown update must be a quiet no-op.
	processor, bot, _ := newProcessor()

	if err := processor.Handle(t.Context(), tg.Update{UpdateID: 3}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if bot.menus != 0 || bot.edits != 0 || len(bot.callbacks) != 0 {
		t.Fatal("an unrecognised update must do nothing")
	}
}

// The processor is what the shared poller and the webhook both drive.
var _ tg.Handler = (*telegraminfra.UpdateProcessor)(nil)
