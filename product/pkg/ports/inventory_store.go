package ports

import (
	"context"
	"errors"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
)

var (
	ErrInventoryItemNotFound = errors.New("not found")
	ErrInsufficientStock     = errors.New("insufficient stock")
	ErrReservationClosed     = errors.New("reservation is not active")
)

// InventoryStore persists stock and reservations, keyed by the canonical
// SkuID (never the human sku string).
type InventoryStore interface {
	GetItem(ctx context.Context, skuID string) (*domain.StockItem, error)
	SaveItem(ctx context.Context, item *domain.StockItem) error
	GetReservation(ctx context.Context, id string) (*domain.Reservation, error)
	SaveReservation(ctx context.Context, reservation *domain.Reservation) error
	CreateReservation(ctx context.Context, orderID string, items []domain.ReservationItem, now time.Time) (*domain.Reservation, error)
	FinalizeReservation(ctx context.Context, id string, status domain.ReservationStatus, now time.Time) (*domain.Reservation, error)
}

// VariantResolver looks up a variant by either identifier. It's the minimal
// slice of ProductStore the inventory service needs to resolve a caller's
// sku/skuId reference to a canonical SkuID before touching InventoryStore.
type VariantResolver interface {
	GetVariant(sku string) (*domain.Variant, error)
	GetVariantBySkuID(skuID string) (*domain.Variant, error)
}
