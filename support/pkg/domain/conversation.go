// Package domain holds the support bot's entities and rules. It depends on
// nothing outside the standard library — not on the Bot API, not on storage.
package domain

import "time"

// DefaultLanguage is the only language the bot answers in at launch. The entry
// language a shopper arrives with is still recorded — see
// docs/support-telegram-bot.md — because it is free to capture now, impossible
// to backfill, and it is the evidence for whether a second language is worth
// staffing.
const DefaultLanguage = "ko"

// Conversation is one Telegram chat's place in the menu.
//
// State is a menu position, not a wizard step: a shopper may type free text at
// any node, and the bot answers rather than insisting on a button.
type Conversation struct {
	ID             string
	ChatID         string
	TelegramUserID *int64
	Username       string
	Language       string
	Node           string
	EntryPayload   string
	LastSeenAt     time.Time
	CreatedAt      time.Time
}

// NewConversation starts a chat at the root menu.
func NewConversation(id, chatID string, now time.Time) *Conversation {
	return &Conversation{
		ID:         id,
		ChatID:     chatID,
		Language:   DefaultLanguage,
		Node:       NodeRoot,
		LastSeenAt: now,
		CreatedAt:  now,
	}
}

// Touch records that the chat was just heard from.
func (c *Conversation) Touch(now time.Time) {
	if c != nil {
		c.LastSeenAt = now
	}
}
