// Package memory holds the in-memory repositories that back local dev and
// tests when DUPLI1_SUPPORT_DB is unset, as order, cart and payment already do.
package memory

import (
	"context"
	"sync"

	"github.com/elug3/dupli1/support/pkg/domain"
)

// ConversationRepository keeps conversations in a map, keyed by chat id.
type ConversationRepository struct {
	mu   sync.RWMutex
	rows map[string]domain.Conversation
}

func NewConversationRepository() *ConversationRepository {
	return &ConversationRepository{rows: make(map[string]domain.Conversation)}
}

func (r *ConversationRepository) FindByChatID(_ context.Context, chatID string) (*domain.Conversation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	row, ok := r.rows[chatID]
	if !ok {
		return nil, nil
	}
	// Copy out: the caller mutates what it gets back, and a pointer into the
	// map would let one request's edit land in the store without a Save.
	return &row, nil
}

func (r *ConversationRepository) Save(_ context.Context, conversation *domain.Conversation) error {
	if conversation == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rows[conversation.ChatID] = *conversation
	return nil
}
