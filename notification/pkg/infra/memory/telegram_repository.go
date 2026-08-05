package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/elug3/dupli1/notification/pkg/domain"
	"github.com/elug3/dupli1/notification/pkg/ports"
	"github.com/jackc/pgx/v4"
	"github.com/oklog/ulid/v2"
)

type TelegramRepository struct {
	mu   sync.RWMutex
	byID map[string]domain.TelegramSubscription
}

func NewTelegramRepository() *TelegramRepository {
	return &TelegramRepository{byID: make(map[string]domain.TelegramSubscription)}
}

func (r *TelegramRepository) UpsertPending(ctx context.Context, in ports.TelegramSubscriptionInput) (*domain.TelegramSubscription, error) {
	_ = ctx
	chatID := strings.TrimSpace(in.ChatID)
	if chatID == "" {
		return nil, fmt.Errorf("chat id is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, sub := range r.byID {
		if sub.ChatID == chatID {
			copy := sub
			return &copy, nil
		}
		if in.TelegramUserID != nil && sub.TelegramUserID != nil && *sub.TelegramUserID == *in.TelegramUserID {
			copy := sub
			return &copy, nil
		}
	}

	now := time.Now().UTC()
	sub := domain.TelegramSubscription{
		ID:             ulid.Make().String(),
		TelegramUserID: in.TelegramUserID,
		ChatID:         chatID,
		ChatType:       strings.TrimSpace(in.ChatType),
		ChatLabel:      strings.TrimSpace(in.ChatLabel),
		Username:       strings.TrimSpace(in.Username),
		Status:         domain.SubscriptionStatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	r.byID[sub.ID] = sub
	copy := sub
	return &copy, nil
}

func (r *TelegramRepository) List(ctx context.Context, status string) ([]domain.TelegramSubscription, error) {
	_ = ctx
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.TelegramSubscription, 0, len(r.byID))
	for _, sub := range r.byID {
		if status == "" || sub.Status == status {
			out = append(out, sub)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (r *TelegramRepository) GetByID(ctx context.Context, id string) (*domain.TelegramSubscription, error) {
	_ = ctx
	r.mu.RLock()
	defer r.mu.RUnlock()
	sub, ok := r.byID[id]
	if !ok {
		return nil, pgx.ErrNoRows
	}
	copy := sub
	return &copy, nil
}

func (r *TelegramRepository) FindByChatID(ctx context.Context, chatID string) (*domain.TelegramSubscription, error) {
	_ = ctx
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, sub := range r.byID {
		if sub.ChatID == strings.TrimSpace(chatID) {
			copy := sub
			return &copy, nil
		}
	}
	return nil, pgx.ErrNoRows
}

func (r *TelegramRepository) FindByUserID(ctx context.Context, userID int64) (*domain.TelegramSubscription, error) {
	_ = ctx
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, sub := range r.byID {
		if sub.TelegramUserID != nil && *sub.TelegramUserID == userID {
			copy := sub
			return &copy, nil
		}
	}
	return nil, pgx.ErrNoRows
}

func (r *TelegramRepository) CreateAccepted(ctx context.Context, in ports.TelegramManualInput) (*domain.TelegramSubscription, error) {
	_ = ctx
	chatID := strings.TrimSpace(in.ChatID)
	if chatID == "" && in.TelegramUserID != nil {
		chatID = fmt.Sprintf("%d", *in.TelegramUserID)
	}
	if chatID == "" {
		return nil, fmt.Errorf("telegram_user_id or chat_id is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	for id, existing := range r.byID {
		if existing.ChatID == chatID {
			existing.Status = domain.SubscriptionStatusAccepted
			existing.AlertOrder = in.AlertOrder
			existing.AlertProduct = in.AlertProduct
			existing.UpdatedAt = now
			existing.AcceptedAt = &now
			existing.AcceptedBy = strings.TrimSpace(in.AcceptedBy)
			if in.TelegramUserID != nil {
				existing.TelegramUserID = in.TelegramUserID
			}
			if in.ChatLabel != "" {
				existing.ChatLabel = in.ChatLabel
			}
			r.byID[id] = existing
			copy := existing
			return &copy, nil
		}
	}

	sub := domain.TelegramSubscription{
		ID:             ulid.Make().String(),
		TelegramUserID: in.TelegramUserID,
		ChatID:         chatID,
		ChatLabel:      strings.TrimSpace(in.ChatLabel),
		Status:         domain.SubscriptionStatusAccepted,
		AlertOrder:     in.AlertOrder,
		AlertProduct:   in.AlertProduct,
		CreatedAt:      now,
		UpdatedAt:      now,
		AcceptedAt:     &now,
		AcceptedBy:     strings.TrimSpace(in.AcceptedBy),
	}
	r.byID[sub.ID] = sub
	copy := sub
	return &copy, nil
}

func (r *TelegramRepository) Accept(ctx context.Context, id string, in ports.TelegramAcceptInput) (*domain.TelegramSubscription, error) {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	sub, ok := r.byID[id]
	if !ok || sub.Status != domain.SubscriptionStatusPending {
		return nil, pgx.ErrNoRows
	}
	now := time.Now().UTC()
	sub.Status = domain.SubscriptionStatusAccepted
	sub.AlertOrder = in.AlertOrder
	sub.AlertProduct = in.AlertProduct
	sub.AcceptedAt = &now
	sub.AcceptedBy = strings.TrimSpace(in.AcceptedBy)
	sub.UpdatedAt = now
	r.byID[id] = sub
	copy := sub
	return &copy, nil
}

func (r *TelegramRepository) Reject(ctx context.Context, id, rejectedBy string) (*domain.TelegramSubscription, error) {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	sub, ok := r.byID[id]
	if !ok || sub.Status != domain.SubscriptionStatusPending {
		return nil, pgx.ErrNoRows
	}
	now := time.Now().UTC()
	sub.Status = domain.SubscriptionStatusRejected
	sub.UpdatedAt = now
	sub.AcceptedBy = strings.TrimSpace(rejectedBy)
	r.byID[id] = sub
	copy := sub
	return &copy, nil
}

func (r *TelegramRepository) Delete(ctx context.Context, id string) error {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[id]; !ok {
		return pgx.ErrNoRows
	}
	delete(r.byID, id)
	return nil
}

func (r *TelegramRepository) ListAccepted(ctx context.Context) ([]domain.TelegramSubscription, error) {
	return r.List(ctx, domain.SubscriptionStatusAccepted)
}
