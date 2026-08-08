package service

import (
	"context"

	"github.com/elug3/dupli1/notification/pkg/domain"
	"github.com/elug3/dupli1/notification/pkg/infra/telegram"
	"github.com/elug3/dupli1/notification/pkg/ports"
)

type subscriptionLookup struct {
	subs *TelegramSubscriptions
}

func NewSubscriptionLookup(subs *TelegramSubscriptions) telegram.SubscriptionLookup {
	if subs == nil || !subs.Enabled() {
		return nil
	}
	return &subscriptionLookup{subs: subs}
}

func (s *subscriptionLookup) RegisterFromMessage(ctx context.Context, in telegram.SubscriptionInput) (*domain.TelegramSubscription, error) {
	return s.subs.RegisterFromMessage(ctx, ports.TelegramSubscriptionInput{
		TelegramUserID: in.TelegramUserID,
		ChatID:         in.ChatID,
		ChatType:       in.ChatType,
		ChatLabel:      in.ChatLabel,
		Username:       in.Username,
	})
}

func (s *subscriptionLookup) FindForMessage(ctx context.Context, chatID string, userID *int64) (*domain.TelegramSubscription, error) {
	return s.subs.LookupForMessage(ctx, chatID, userID)
}
