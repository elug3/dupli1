package ports

import (
	"context"
	"errors"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	NextOrderID(ctx context.Context) (string, error)
	Save(ctx context.Context, order *domain.Order) error
	// SaveWithOutbox persists the order, optional idempotency row, and outbox events in one transaction.
	SaveWithOutbox(ctx context.Context, order *domain.Order, idem *IdempotencyRecord, events []OutboxEvent) error
	Get(ctx context.Context, id string) (*domain.Order, error)
	ListByCustomer(ctx context.Context, customerID string) ([]domain.Order, error)
	ListPendingPaymentExpired(ctx context.Context, now time.Time) ([]domain.Order, error)
	NextCheckoutSessionID(ctx context.Context) (string, error)
	SaveCheckoutSession(ctx context.Context, session *domain.CheckoutSession) error
	GetCheckoutSession(ctx context.Context, id string) (*domain.CheckoutSession, error)

	FindByIdempotencyKey(ctx context.Context, customerID, key string) (*IdempotencyRecord, error)
	ListPendingOutbox(ctx context.Context, limit int) ([]OutboxMessage, error)
	MarkOutboxPublished(ctx context.Context, id int64) error
	RecordOutboxAttempt(ctx context.Context, id int64, errMsg string) error
}
