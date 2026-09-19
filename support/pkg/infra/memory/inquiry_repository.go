package memory

import (
	"context"
	"sync"

	"github.com/elug3/dupli1/support/pkg/domain"
)

// InquiryRepository keeps escalations in a map.
type InquiryRepository struct {
	mu   sync.RWMutex
	rows map[string]domain.Inquiry
}

func NewInquiryRepository() *InquiryRepository {
	return &InquiryRepository{rows: make(map[string]domain.Inquiry)}
}

func (r *InquiryRepository) FindOpenByChatID(_ context.Context, chatID string) (*domain.Inquiry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, row := range r.rows {
		if row.ChatID == chatID && row.IsOpen() {
			found := row
			return &found, nil
		}
	}
	return nil, nil
}

func (r *InquiryRepository) Save(_ context.Context, inquiry *domain.Inquiry) error {
	if inquiry == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rows[inquiry.ID] = *inquiry
	return nil
}

// MessageRepository keeps the conversation transcript in memory.
type MessageRepository struct {
	mu   sync.RWMutex
	rows []domain.Message
}

func NewMessageRepository() *MessageRepository {
	return &MessageRepository{}
}

func (r *MessageRepository) Append(_ context.Context, message *domain.Message) error {
	if message == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rows = append(r.rows, *message)
	return nil
}

// LastInbound returns the most recent thing the shopper typed.
func (r *MessageRepository) LastInbound(_ context.Context, conversationID string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i := len(r.rows) - 1; i >= 0; i-- {
		row := r.rows[i]
		if row.ConversationID == conversationID && row.Direction == domain.DirectionInbound {
			return row.Body, nil
		}
	}
	return "", nil
}
