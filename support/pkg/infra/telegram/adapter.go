// Package telegram adapts the shared Bot API client to the support service's
// ports: Bot API updates become domain-shaped intents on the way in, and menu
// buttons become inline keyboards on the way out.
package telegram

import (
	"context"

	tg "github.com/elug3/dupli1/shared/pkg/telegram"
	"github.com/elug3/dupli1/support/pkg/ports"
	"github.com/elug3/dupli1/support/pkg/service"
)

// Bot implements ports.Bot over the shared client.
type Bot struct {
	Client *tg.Client
}

func (b *Bot) ReplyMenu(ctx context.Context, chatID string, text string, buttons []ports.MenuButton) error {
	if b == nil || b.Client == nil {
		return nil
	}
	rendered := make([]tg.InlineKeyboardButton, 0, len(buttons))
	for _, button := range buttons {
		rendered = append(rendered, tg.CallbackButton(button.Label, button.CallbackData))
	}
	return b.Client.ReplyMenu(ctx, chatID, text, tg.KeyboardRows(rendered...))
}

func (b *Bot) AnswerCallback(ctx context.Context, callbackQueryID string, text string) error {
	if b == nil || b.Client == nil {
		return nil
	}
	return b.Client.AnswerCallback(ctx, callbackQueryID, text)
}

// UpdateProcessor turns a Bot API update into one service call. It satisfies
// tg.Handler, so the shared poller drives it in local dev exactly as the
// webhook does in production.
type UpdateProcessor struct {
	Router *service.Router
}

// Handle routes a message or a button tap. Any other update type is ignored:
// the webhook only asks for these two, but polling delivers more.
func (p *UpdateProcessor) Handle(ctx context.Context, update tg.Update) error {
	if p == nil || p.Router == nil {
		return nil
	}
	in, ok := inboundFrom(update)
	if !ok {
		return nil
	}
	return p.Router.Handle(ctx, in)
}

func inboundFrom(update tg.Update) (service.Inbound, bool) {
	switch {
	case update.CallbackQuery != nil:
		query := update.CallbackQuery
		chat := query.Chat()
		in := service.Inbound{
			ChatID:          chat.FormatID(),
			ChatType:        chat.Type,
			CallbackQueryID: query.ID,
			CallbackData:    query.Data,
		}
		if query.From != nil {
			id := query.From.ID
			in.TelegramUserID = &id
			in.Username = query.From.Username
		}
		return in, true

	case update.Message != nil:
		msg := update.Message
		in := service.Inbound{
			ChatID:   msg.Chat.FormatID(),
			ChatType: msg.Chat.Type,
			Text:     msg.Text,
		}
		if msg.From != nil {
			id := msg.From.ID
			in.TelegramUserID = &id
			in.Username = msg.From.Username
		}
		return in, true
	}
	return service.Inbound{}, false
}
