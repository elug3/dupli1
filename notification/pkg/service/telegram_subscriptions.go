package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/elug3/dupli1/notification/pkg/domain"
	"github.com/elug3/dupli1/notification/pkg/ports"
	"github.com/jackc/pgx/v4"
)

type TelegramSubscriptions struct {
	repo ports.TelegramRepository
}

func NewTelegramSubscriptions(repo ports.TelegramRepository) *TelegramSubscriptions {
	return &TelegramSubscriptions{repo: repo}
}

func (s *TelegramSubscriptions) Enabled() bool {
	return s != nil && s.repo != nil
}

func (s *TelegramSubscriptions) RegisterFromMessage(ctx context.Context, in ports.TelegramSubscriptionInput) (*domain.TelegramSubscription, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("telegram repository not configured")
	}
	return s.repo.UpsertPending(ctx, in)
}

func (s *TelegramSubscriptions) List(ctx context.Context, status string) ([]domain.TelegramSubscription, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("telegram repository not configured")
	}
	return s.repo.List(ctx, status)
}

func (s *TelegramSubscriptions) CreateManual(ctx context.Context, in ports.TelegramManualInput) (*domain.TelegramSubscription, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("telegram repository not configured")
	}
	if in.TelegramUserID == nil && strings.TrimSpace(in.ChatID) == "" {
		return nil, fmt.Errorf("telegram_user_id or chat_id is required")
	}
	return s.repo.CreateAccepted(ctx, in)
}

func (s *TelegramSubscriptions) Accept(ctx context.Context, id string, in ports.TelegramAcceptInput) (*domain.TelegramSubscription, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("telegram repository not configured")
	}
	return s.repo.Accept(ctx, id, in)
}

func (s *TelegramSubscriptions) Reject(ctx context.Context, id, rejectedBy string) (*domain.TelegramSubscription, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("telegram repository not configured")
	}
	return s.repo.Reject(ctx, id, rejectedBy)
}

func (s *TelegramSubscriptions) Delete(ctx context.Context, id string) error {
	if !s.Enabled() {
		return fmt.Errorf("telegram repository not configured")
	}
	return s.repo.Delete(ctx, id)
}

func (s *TelegramSubscriptions) LookupForMessage(ctx context.Context, chatID string, userID *int64) (*domain.TelegramSubscription, error) {
	if !s.Enabled() {
		return nil, nil
	}
	if sub, err := s.repo.FindByChatID(ctx, chatID); err == nil {
		return sub, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if userID != nil {
		if sub, err := s.repo.FindByUserID(ctx, *userID); err == nil {
			return sub, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	return nil, nil
}

func (s *TelegramSubscriptions) IsAllowedIncoming(ctx context.Context, chatID string, userID *int64, env *ports.TelegramEnvAllowlist) bool {
	if userID != nil && env != nil && env.AllowsUser(*userID) {
		return true
	}
	if env != nil && env.AllowsChat(chatID) {
		return true
	}
	sub, err := s.LookupForMessage(ctx, chatID, userID)
	if err != nil || sub == nil {
		return false
	}
	return sub.IsAccepted()
}

// RoutingChats returns every chat that should receive order and product
// alerts: the transitional env destinations first, then each accepted
// subscription carrying the matching flag, without duplicates.
//
// Env and database destinations are unioned rather than one overriding the
// other. Preferring env meant that as long as TELEGRAM_ORDER_CHAT_ID was set —
// which it is in production — accepting a chat in manage-web did nothing at
// all, so the whole subscription UI was inert wherever the fallback existed.
func (s *TelegramSubscriptions) RoutingChats(ctx context.Context, env *ports.TelegramEnvAllowlist) (orderChatIDs, productChatIDs []string) {
	var order, product chatSet
	if env != nil {
		order.add(env.OrderChatID)
		product.add(env.ProductChatID)
	}
	if !s.Enabled() {
		return order.list(), product.list()
	}
	accepted, err := s.repo.ListAccepted(ctx)
	if err != nil {
		return order.list(), product.list()
	}
	for _, sub := range accepted {
		if sub.AlertOrder {
			order.add(sub.ChatID)
		}
		if sub.AlertProduct {
			product.add(sub.ChatID)
		}
	}
	return order.list(), product.list()
}

// chatSet collects chat IDs in insertion order, trimming blanks and repeats so
// a chat listed in both env and the database is alerted once.
type chatSet struct {
	ids  []string
	seen map[string]struct{}
}

func (c *chatSet) add(chatID string) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return
	}
	if _, ok := c.seen[chatID]; ok {
		return
	}
	if c.seen == nil {
		c.seen = make(map[string]struct{})
	}
	c.seen[chatID] = struct{}{}
	c.ids = append(c.ids, chatID)
}

func (c *chatSet) list() []string { return c.ids }

func (s *TelegramSubscriptions) AllowedChatIDs(ctx context.Context, env *ports.TelegramEnvAllowlist) (map[string]struct{}, error) {
	allowed := make(map[string]struct{})
	if env != nil {
		for _, id := range env.ChatIDs() {
			allowed[id] = struct{}{}
		}
	}
	if !s.Enabled() {
		return allowed, nil
	}
	accepted, err := s.repo.ListAccepted(ctx)
	if err != nil {
		return nil, err
	}
	for _, sub := range accepted {
		allowed[sub.ChatID] = struct{}{}
	}
	return allowed, nil
}
