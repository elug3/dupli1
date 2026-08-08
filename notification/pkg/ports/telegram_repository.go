package ports

import (
	"context"

	"github.com/elug3/dupli1/notification/pkg/domain"
)

// TelegramSubscriptionInput captures a registration from an inbound Telegram update.
type TelegramSubscriptionInput struct {
	TelegramUserID *int64
	ChatID         string
	ChatType       string
	ChatLabel      string
	Username       string
}

// TelegramManualInput is a manager-created subscription.
type TelegramManualInput struct {
	TelegramUserID *int64
	ChatID         string
	ChatLabel      string
	AlertOrder     bool
	AlertProduct   bool
	AcceptedBy     string
}

// TelegramAcceptInput updates routing flags when a manager accepts a subscription.
type TelegramAcceptInput struct {
	AlertOrder   bool
	AlertProduct bool
	AcceptedBy   string
}

// TelegramRepository persists Telegram allowlist entries.
type TelegramRepository interface {
	UpsertPending(ctx context.Context, in TelegramSubscriptionInput) (*domain.TelegramSubscription, error)
	List(ctx context.Context, status string) ([]domain.TelegramSubscription, error)
	GetByID(ctx context.Context, id string) (*domain.TelegramSubscription, error)
	FindByChatID(ctx context.Context, chatID string) (*domain.TelegramSubscription, error)
	FindByUserID(ctx context.Context, userID int64) (*domain.TelegramSubscription, error)
	CreateAccepted(ctx context.Context, in TelegramManualInput) (*domain.TelegramSubscription, error)
	Accept(ctx context.Context, id string, in TelegramAcceptInput) (*domain.TelegramSubscription, error)
	Reject(ctx context.Context, id, rejectedBy string) (*domain.TelegramSubscription, error)
	Delete(ctx context.Context, id string) error
	ListAccepted(ctx context.Context) ([]domain.TelegramSubscription, error)
}
