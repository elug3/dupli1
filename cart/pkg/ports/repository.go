package ports

import (
	"context"
	"errors"
	"time"

	"github.com/elug3/dupli1/cart/pkg/domain"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	GetItems(ctx context.Context, customerID string) ([]domain.StoredItem, time.Time, error)
	ReplaceItems(ctx context.Context, customerID string, items []domain.StoredItem, updatedAt time.Time) error
	UpsertItem(ctx context.Context, customerID string, item domain.StoredItem, updatedAt time.Time) error
	RemoveItem(ctx context.Context, customerID, sku string, updatedAt time.Time) error
	RemoveItemBySkuID(ctx context.Context, customerID, skuID string, updatedAt time.Time) error
	Clear(ctx context.Context, customerID string) error
}
