// Package telegram adapts the shared Bot API client to the support service's
// ports: Bot API updates become domain-shaped intents on the way in, and menu
// buttons become inline keyboards on the way out.
package telegram

import (
	"context"
	"strings"

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
	return b.Client.ReplyMenu(ctx, chatID, text, keyboard(buttons))
}

// Reply sends plain text with no keyboard — a manager's words, as typed.
func (b *Bot) Reply(ctx context.Context, chatID string, text string) error {
	if b == nil || b.Client == nil {
		return nil
	}
	return b.Client.Reply(ctx, chatID, text)
}

// EditMenu replaces an earlier message in place.
//
// Telegram answers a re-tap of the button already open with 400 "message is not
// modified", because the new content is identical to the old. Nothing is wrong
// — the shopper is already looking at what they asked for — so it is not worth
// an error line in the log on an interaction people make all the time.
func (b *Bot) EditMenu(ctx context.Context, chatID string, messageID int64, text string, buttons []ports.MenuButton) error {
	if b == nil || b.Client == nil {
		return nil
	}
	err := b.Client.EditMessageText(ctx, chatID, messageID, text, keyboard(buttons))
	if err != nil && strings.Contains(err.Error(), "message is not modified") {
		return nil
	}
	return err
}

func (b *Bot) AnswerCallback(ctx context.Context, callbackQueryID string, text string) error {
	if b == nil || b.Client == nil {
		return nil
	}
	return b.Client.AnswerCallback(ctx, callbackQueryID, text)
}

func keyboard(buttons []ports.MenuButton) *tg.InlineKeyboardMarkup {
	rendered := make([]tg.InlineKeyboardButton, 0, len(buttons))
	for _, button := range buttons {
		rendered = append(rendered, tg.CallbackButton(button.Label, button.CallbackData))
	}
	return tg.KeyboardRows(rendered...)
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
		if query.Message != nil {
			// The id of the message the button hangs under — what the menu is
			// edited in place by.
			in.MessageID = query.Message.MessageID
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
