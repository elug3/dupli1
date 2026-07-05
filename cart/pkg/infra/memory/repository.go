package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/elug3/dupli1/cart/pkg/domain"
)

type cartRecord struct {
	items     []domain.StoredItem
	updatedAt time.Time
}

type Repository struct {
	mu    sync.RWMutex
	carts map[string]*cartRecord
}

func NewRepository() *Repository {
	return &Repository{carts: make(map[string]*cartRecord)}
}

func (r *Repository) GetItems(ctx context.Context, customerID string) ([]domain.StoredItem, time.Time, error) {
	if err := ctx.Err(); err != nil {
		return nil, time.Time{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	record, ok := r.carts[customerID]
	if !ok {
		return []domain.StoredItem{}, time.Time{}, nil
	}
	return cloneItems(record.items), record.updatedAt, nil
}

func (r *Repository) ReplaceItems(ctx context.Context, customerID string, items []domain.StoredItem, updatedAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.carts[customerID] = &cartRecord{
		items:     cloneItems(items),
		updatedAt: updatedAt,
	}
	return nil
}

func (r *Repository) UpsertItem(ctx context.Context, customerID string, item domain.StoredItem, updatedAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	record := r.carts[customerID]
	if record == nil {
		record = &cartRecord{items: []domain.StoredItem{}}
		r.carts[customerID] = record
	}

	for i, existing := range record.items {
		if existing.SKU == item.SKU {
			record.items[i] = item
			record.updatedAt = updatedAt
			return nil
		}
	}
	record.items = append(record.items, item)
	sort.Slice(record.items, func(i, j int) bool {
		return record.items[i].SKU < record.items[j].SKU
	})
	record.updatedAt = updatedAt
	return nil
}

func (r *Repository) RemoveItem(ctx context.Context, customerID, sku string, updatedAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	sku = domain.NormalizeSKU(sku)
	r.mu.Lock()
	defer r.mu.Unlock()

	record, ok := r.carts[customerID]
	if !ok {
		return nil
	}

	filtered := record.items[:0]
	for _, item := range record.items {
		if item.SKU != sku {
			filtered = append(filtered, item)
		}
	}
	record.items = filtered
	record.updatedAt = updatedAt
	return nil
}

func (r *Repository) Clear(ctx context.Context, customerID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.carts, customerID)
	return nil
}

func cloneItems(items []domain.StoredItem) []domain.StoredItem {
	if len(items) == 0 {
		return []domain.StoredItem{}
	}
	out := make([]domain.StoredItem, len(items))
	copy(out, items)
	return out
}
