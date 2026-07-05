package ports

import (
	"context"
	"errors"

	"github.com/elug3/dupli1/payment/pkg/domain"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	NextPaymentID(ctx context.Context) (string, error)
	Save(ctx context.Context, payment *domain.Payment) error
	Get(ctx context.Context, id string) (*domain.Payment, error)
	GetByProviderRef(ctx context.Context, providerRef string) (*domain.Payment, error)
	FindByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error)
}
