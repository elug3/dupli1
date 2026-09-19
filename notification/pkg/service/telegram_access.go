package service

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/elug3/dupli1/notification/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/telegram"
)

// DefaultAccessRefreshInterval is how often the cached allowlist is rebuilt
// from the database when no interval is configured.
const DefaultAccessRefreshInterval = 30 * time.Second

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

// RunRefresher rebuilds the cached allowlist on a ticker until ctx is done.
//
// The handler's OnSubscriptionsChanged callback only refreshes the process that
// served the request, so without this a chat a manager accepts on one task
// stays denied on every other one until it restarts — including through the
// two-task overlap of a rolling deploy.
func (a *TelegramAccess) RunRefresher(ctx context.Context, every time.Duration) {
	if a == nil {
		return
	}
	if every <= 0 {
		every = DefaultAccessRefreshInterval
	}

	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.Refresh(ctx); err != nil && ctx.Err() == nil {
				log.Printf("telegram access refresh: %v", err)
			}
		}
	}
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
