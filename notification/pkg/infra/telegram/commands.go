package telegram

import (
	"fmt"
	"strings"
)

// IsStartCommand reports whether text is a /start command (optionally with @bot suffix).
func IsStartCommand(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}

	cmd := strings.Fields(text)[0]
	if at := strings.Index(cmd, "@"); at >= 0 {
		cmd = cmd[:at]
	}
	return cmd == "/start"
}

// FormatPendingReply is sent when a user is registered but not yet accepted.
func FormatPendingReply(chat Chat) string {
	return fmt.Sprintf(
		"⏳ <b>Registration received</b>\n\n"+
			"%s\n"+
			"Chat ID: <code>%s</code>\n\n"+
			"A manager must approve this chat before alerts are enabled. "+
			"You will be able to use /start again after approval.",
		chatLabel(chat),
		escapeHTML(chat.FormatID()),
	)
}

// FormatStartReply returns the welcome message for /start, including the chat ID for ops setup.
func FormatStartReply(chat Chat) string {
	chatID := chat.FormatID()
	chatLabel := chatLabel(chat)

	return fmt.Sprintf(
		"👋 <b>Welcome to Dupli1 ops alerts</b>\n\n"+
			"This bot sends order and product notifications from the Dupli1 marketplace.\n\n"+
			"%s\n"+
			"Chat ID: <code>%s</code>\n\n"+
			"You are on the approved list and can receive ops alerts from this chat.",
		chatLabel,
		escapeHTML(chatID),
	)
}

func chatLabel(chat Chat) string {
	switch strings.TrimSpace(chat.Type) {
	case "private":
		name := strings.TrimSpace(chat.FirstName)
		if name == "" {
			name = strings.TrimSpace(chat.Username)
		}
		if name != "" {
			return fmt.Sprintf("Chat: <b>%s</b> (private)", escapeHTML(name))
		}
		return "Chat: <b>private</b>"
	case "group", "supergroup":
		title := strings.TrimSpace(chat.Title)
		if title != "" {
			return fmt.Sprintf("Group: <b>%s</b>", escapeHTML(title))
		}
		return "Group chat"
	case "channel":
		title := strings.TrimSpace(chat.Title)
		if title != "" {
			return fmt.Sprintf("Channel: <b>%s</b>", escapeHTML(title))
		}
		return "Channel"
	default:
		return "Chat"
	}
}

func escapeHTML(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(strings.TrimSpace(value))
}
