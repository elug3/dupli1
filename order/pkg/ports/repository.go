package ports

import (
	"context"
	"errors"

	"github.com/elug3/schick/pkg/order/domain"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	NextOrderID(ctx context.Context) (string, error)
	Save(ctx context.Context, order *domain.Order) error
	Get(ctx context.Context, id string) (*domain.Order, error)
	ListByCustomer(ctx context.Context, customerID string) ([]domain.Order, error)
}
