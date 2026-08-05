package service

import (
	"context"
	"strconv"
	"sync"

	"github.com/elug3/dupli1/notification/pkg/infra/telegram"
	"github.com/elug3/dupli1/notification/pkg/ports"
)

// TelegramAccess enforces Telegram send and command policy from env + database.
type TelegramAccess struct {
	mu   sync.RWMutex
	subs *TelegramSubscriptions
	env  *ports.TelegramEnvAllowlist

	chatIDs map[string]struct{}
	userIDs map[string]struct{}
}

func NewTelegramAccess(subs *TelegramSubscriptions, env *ports.TelegramEnvAllowlist) *TelegramAccess {
	return &TelegramAccess{
		subs:    subs,
		env:     env,
		chatIDs: make(map[string]struct{}),
		userIDs: make(map[string]struct{}),
	}
}

func (a *TelegramAccess) Refresh(ctx context.Context) error {
	if a == nil {
		return nil
	}
	chatIDs := make(map[string]struct{})
	userIDs := make(map[string]struct{})

	if a.env != nil {
		for _, id := range a.env.ChatIDs() {
			chatIDs[id] = struct{}{}
		}
		for _, id := range a.env.AllowedUserIDList() {
			userIDs[id] = struct{}{}
		}
	}

	if a.subs != nil && a.subs.Enabled() {
		accepted, err := a.subs.repo.ListAccepted(ctx)
		if err != nil {
			return err
		}
		for _, sub := range accepted {
			chatIDs[sub.ChatID] = struct{}{}
			if sub.TelegramUserID != nil {
				userIDs[strconv.FormatInt(*sub.TelegramUserID, 10)] = struct{}{}
			}
		}
	}

	a.mu.Lock()
	a.chatIDs = chatIDs
	a.userIDs = userIDs
	a.mu.Unlock()
	return nil
}

func (a *TelegramAccess) AllowsChat(chatID string) bool {
	if a == nil {
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.chatIDs[chatID]
	return ok
}

func (a *TelegramAccess) AllowsIncoming(chat telegram.Chat, from *telegram.User) bool {
	if a == nil {
		return false
	}
	if a.AllowsChat(chat.FormatID()) {
		return true
	}
	if from != nil {
		a.mu.RLock()
		_, ok := a.userIDs[strconv.FormatInt(from.ID, 10)]
		a.mu.RUnlock()
		return ok
	}
	return false
}
