package telegram

// InlineKeyboardButton is one tappable button under a message.
//
// A button carries either CallbackData (the tap comes back as a CallbackQuery
// update) or URL (the tap opens a link and the bot hears nothing). Telegram
// caps CallbackData at 64 bytes, which is why menu routing encodes a short
// node id rather than prose — see docs/support-telegram-bot.md.
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

// InlineKeyboardMarkup is the grid of buttons attached to a message: an outer
// slice of rows, each holding the buttons shown side by side on that row.
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// KeyboardRows builds a keyboard one button per row, which is the readable
// shape for a consultation menu on a phone.
func KeyboardRows(buttons ...InlineKeyboardButton) *InlineKeyboardMarkup {
	if len(buttons) == 0 {
		return nil
	}
	rows := make([][]InlineKeyboardButton, 0, len(buttons))
	for _, button := range buttons {
		rows = append(rows, []InlineKeyboardButton{button})
	}
	return &InlineKeyboardMarkup{InlineKeyboard: rows}
}

// CallbackButton is shorthand for a button that routes back to the bot.
func CallbackButton(text, data string) InlineKeyboardButton {
	return InlineKeyboardButton{Text: text, CallbackData: data}
}

// messagePayload is the sendMessage / editMessageText request body. Before
// menus this was a map[string]string; it is a struct so reply_markup can be
// present when a keyboard is attached and absent when it is not — omitempty
// keeps the wire format byte-identical for plain messages.
type messagePayload struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
	MessageID int64  `json:"message_id,omitempty"`
	// DisableNotification delivers the message without a sound or vibration.
	// Telegram still shows it; it simply does not interrupt.
	DisableNotification bool                  `json:"disable_notification,omitempty"`
	ReplyMarkup         *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}
