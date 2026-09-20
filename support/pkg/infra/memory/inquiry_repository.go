package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/elug3/dupli1/support/pkg/ports"

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

func (r *InquiryRepository) FindByID(_ context.Context, id string) (*domain.Inquiry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	row, ok := r.rows[id]
	if !ok {
		return nil, nil
	}
	found := row
	return &found, nil
}

func (r *InquiryRepository) List(_ context.Context, filter ports.InquiryFilter) ([]domain.Inquiry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []domain.Inquiry
	for _, row := range r.rows {
		if filter.Status != "" && row.Status != filter.Status {
			continue
		}
		if filter.AssignedTo != "" && row.AssignedTo != filter.AssignedTo {
			continue
		}
		if filter.Unassigned && (row.AssignedTo != "" || !row.IsOpen()) {
			continue
		}
		out = append(out, row)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].OpenedAt.After(out[b].OpenedAt) })
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

func (r *InquiryRepository) ListStaleOpen(_ context.Context, quietSince time.Time) ([]domain.Inquiry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []domain.Inquiry
	for _, row := range r.rows {
		if row.IsOpen() && row.OpenedAt.Before(quietSince) {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].OpenedAt.Before(out[b].OpenedAt) })
	return out, nil
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

// Transcript returns a conversation's messages, oldest first.
func (r *MessageRepository) Transcript(_ context.Context, conversationID string) ([]domain.Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []domain.Message
	for _, row := range r.rows {
		if row.ConversationID == conversationID {
			out = append(out, row)
		}
	}
	return out, nil
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

// PurgeBodies replaces the text of messages older than the cutoff.
func (r *MessageRepository) PurgeBodies(_ context.Context, olderThan time.Time, placeholder string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	purged := 0
	for i := range r.rows {
		if r.rows[i].CreatedAt.Before(olderThan) && r.rows[i].Body != placeholder {
			r.rows[i].Body = placeholder
			r.rows[i].DeliveryError = ""
			purged++
		}
	}
	return purged, nil
}
