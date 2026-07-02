package ports

import (
	"context"
	"errors"
	"time"

	"github.com/elug3/dupli1/inventory/pkg/domain"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrInsufficientStock   = errors.New("insufficient stock")
	ErrReservationClosed   = errors.New("reservation is not active")
)

type Repository interface {
	GetItem(ctx context.Context, sku string) (*domain.StockItem, error)
	SaveItem(ctx context.Context, item *domain.StockItem) error
	GetReservation(ctx context.Context, id string) (*domain.Reservation, error)
	SaveReservation(ctx context.Context, reservation *domain.Reservation) error
	CreateReservation(ctx context.Context, orderID string, items []domain.ReservationItem, now time.Time) (*domain.Reservation, error)
	FinalizeReservation(ctx context.Context, id string, status domain.ReservationStatus, now time.Time) (*domain.Reservation, error)
}
