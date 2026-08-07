package telegram

import (
	"strconv"
	"strings"
)

// Allowlist restricts which Telegram users and chats may receive messages.
type Allowlist struct {
	chatIDs map[string]struct{}
	userIDs map[string]struct{}
}

// NewAllowlist builds an allowlist from configured order/product chat IDs and
// an optional comma-separated list of Telegram user IDs.
func NewAllowlist(orderChatID, productChatID, allowedUserIDs string) *Allowlist {
	a := &Allowlist{
		chatIDs: make(map[string]struct{}),
		userIDs: make(map[string]struct{}),
	}
	for _, id := range []string{orderChatID, productChatID} {
		if id = strings.TrimSpace(id); id != "" {
			a.chatIDs[id] = struct{}{}
		}
	}
	for _, id := range parseIDList(allowedUserIDs) {
		a.userIDs[id] = struct{}{}
	}
	return a
}

func parseIDList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var ids []string
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	}) {
		part = strings.TrimSpace(part)
		if part != "" {
			ids = append(ids, part)
		}
	}
	return ids
}

func (a *Allowlist) AllowsChat(chatID string) bool {
	if a == nil {
		return false
	}
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return false
	}
	_, ok := a.chatIDs[chatID]
	return ok
}

func (a *Allowlist) AllowsUser(userID int64) bool {
	if a == nil || userID == 0 {
		return false
	}
	_, ok := a.userIDs[strconv.FormatInt(userID, 10)]
	return ok
}

// AllowsIncoming reports whether an incoming command may be handled for the sender.
func (a *Allowlist) AllowsIncoming(chat Chat, from *User) bool {
	if a.AllowsChat(chat.FormatID()) {
		return true
	}
	if from != nil {
		return a.AllowsUser(from.ID)
	}
	return false
}
