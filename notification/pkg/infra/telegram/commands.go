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
		"⏳ <b>등록 요청이 접수되었습니다</b>\n\n"+
			"%s\n"+
			"채팅 ID: <code>%s</code>\n\n"+
			"관리자가 이 채팅을 승인해야 알림이 활성화됩니다. "+
			"승인 후 /start를 다시 사용할 수 있습니다.",
		chatLabel(chat),
		escapeHTML(chat.FormatID()),
	)
}

// FormatStartReply returns the welcome message for /start, including the chat ID for ops setup.
func FormatStartReply(chat Chat) string {
	chatID := chat.FormatID()
	chatLabel := chatLabel(chat)

	return fmt.Sprintf(
		"👋 <b>Dupli1 운영 알림</b>\n\n"+
			"이 봇은 Dupli1 마켓플레이스의 주문·상품 알림을 보냅니다.\n\n"+
			"%s\n"+
			"채팅 ID: <code>%s</code>\n\n"+
			"승인된 채팅이므로 이 채팅에서 운영 알림을 받을 수 있습니다.",
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
			return fmt.Sprintf("개인 채팅: <b>%s</b>", escapeHTML(name))
		}
		return "개인 채팅"
	case "group", "supergroup":
		title := strings.TrimSpace(chat.Title)
		if title != "" {
			return fmt.Sprintf("그룹: <b>%s</b>", escapeHTML(title))
		}
		return "그룹 채팅"
	case "channel":
		title := strings.TrimSpace(chat.Title)
		if title != "" {
			return fmt.Sprintf("채널: <b>%s</b>", escapeHTML(title))
		}
		return "채널"
	default:
		return "채팅"
	}
}

// escapeHTML escapes the characters Telegram's HTML parse mode treats as
// markup, quotes included so the same helper is safe in an attribute as well as
// in text. Kept in step with the copy in pkg/service.
func escapeHTML(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(strings.TrimSpace(value))
}
