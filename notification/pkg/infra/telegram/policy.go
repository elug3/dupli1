package telegram

// AccessPolicy controls which Telegram chats and users may receive messages or commands.
type AccessPolicy interface {
	AllowsChat(chatID string) bool
	AllowsIncoming(chat Chat, from *User) bool
}
