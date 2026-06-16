package ports

import (
	"context"
	"errors"

	"github.com/elug3/schick/pkg/inventory/domain"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	NextReservationID(ctx context.Context) (string, error)
	GetItem(ctx context.Context, sku string) (*domain.StockItem, error)
	SaveItem(ctx context.Context, item *domain.StockItem) error
	GetReservation(ctx context.Context, id string) (*domain.Reservation, error)
	SaveReservation(ctx context.Context, reservation *domain.Reservation) error
}
