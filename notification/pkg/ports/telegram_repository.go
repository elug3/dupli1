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

// ErrSubscriptionRejected reports a write to a rejected subscription's alert
// flags. A rejected chat is refused everything, so its flags would route
// nothing; the manager deletes it, or the chat sends /start to ask again.
var ErrSubscriptionRejected = errors.New("telegram subscription is rejected")

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
	AlertSupport   bool
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
	AlertSupport bool
	AcceptedBy   string
}

// TelegramAlertsInput changes the alert classes a subscription receives. A nil
// flag is left as it is, so a caller changing one class cannot clear another
// it never sent.
type TelegramAlertsInput struct {
	AlertOrder   *bool
	AlertProduct *bool
	AlertSupport *bool
}

// Empty reports whether the input changes nothing.
func (in TelegramAlertsInput) Empty() bool {
	return in.AlertOrder == nil && in.AlertProduct == nil && in.AlertSupport == nil
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
	// UpdateAlerts sets the given alert flags on a pending or accepted
	// subscription: pgx.ErrNoRows when it does not exist,
	// ErrSubscriptionRejected when it is rejected.
	UpdateAlerts(ctx context.Context, id string, in TelegramAlertsInput) (*domain.TelegramSubscription, error)
	Delete(ctx context.Context, id string) error
	ListAccepted(ctx context.Context) ([]domain.TelegramSubscription, error)
}
