package ports

import (
	"strconv"
	"strings"
)

// TelegramEnvAllowlist is the transitional env-based allowlist.
type TelegramEnvAllowlist struct {
	OrderChatID    string
	ProductChatID  string
	AllowedUserIDs string
}

func (e *TelegramEnvAllowlist) ChatIDs() []string {
	if e == nil {
		return nil
	}
	var ids []string
	for _, id := range []string{strings.TrimSpace(e.OrderChatID), strings.TrimSpace(e.ProductChatID)} {
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func (e *TelegramEnvAllowlist) AllowsChat(chatID string) bool {
	chatID = strings.TrimSpace(chatID)
	for _, id := range e.ChatIDs() {
		if id == chatID {
			return true
		}
	}
	return false
}

func (e *TelegramEnvAllowlist) AllowedUserIDList() []string {
	return parseIDList(e.AllowedUserIDs)
}

func (e *TelegramEnvAllowlist) AllowsUser(userID int64) bool {
	if e == nil || userID == 0 {
		return false
	}
	target := strconv.FormatInt(userID, 10)
	for _, part := range parseIDList(e.AllowedUserIDs) {
		if part == target {
			return true
		}
	}
	return false
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
