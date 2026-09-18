package ports

import (
	"context"
	"errors"

	"github.com/elug3/dupli1/notification/pkg/domain"
)

// ErrDuplicateSubscription reports that another row already claims the chat ID
// or Telegram user ID being written. It is a caller mistake, not a failure of
// the store, and callers map it to a conflict rather than a server error.
var ErrDuplicateSubscription = errors.New("telegram subscription already exists")

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

// TelegramMetadataInput carries the display fields an inbound message can
// refresh on a subscription that already exists.
type TelegramMetadataInput struct {
	ChatType  string
	ChatLabel string
	Username  string
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
	UpdateMetadata(ctx context.Context, id string, in TelegramMetadataInput) error
	CreateAccepted(ctx context.Context, in TelegramManualInput) (*domain.TelegramSubscription, error)
	Accept(ctx context.Context, id string, in TelegramAcceptInput) (*domain.TelegramSubscription, error)
	Reject(ctx context.Context, id, rejectedBy string) (*domain.TelegramSubscription, error)
	Delete(ctx context.Context, id string) error
	ListAccepted(ctx context.Context) ([]domain.TelegramSubscription, error)
}
