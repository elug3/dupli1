package ports

import (
	"context"
	"errors"
	"time"

	"github.com/elug3/dupli1/payment/pkg/domain"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	NextPaymentID(ctx context.Context) (string, error)
	Save(ctx context.Context, payment *domain.Payment) error
	// SaveWithOutbox persists the payment and outbox events in one transaction.
	SaveWithOutbox(ctx context.Context, payment *domain.Payment, events []OutboxEvent) error
	Get(ctx context.Context, id string) (*domain.Payment, error)
	GetByProviderRef(ctx context.Context, providerRef string) (*domain.Payment, error)
	FindByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error)
	ListSucceededSince(ctx context.Context, since time.Time, limit int) ([]domain.Payment, error)

	ListPendingOutbox(ctx context.Context, limit int) ([]OutboxMessage, error)
	MarkOutboxPublished(ctx context.Context, id int64) error
	RecordOutboxAttempt(ctx context.Context, id int64, errMsg string) error
}
