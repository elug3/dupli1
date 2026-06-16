package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/elug3/schick/pkg/inventory/domain"
	"github.com/elug3/schick/pkg/inventory/ports"
)

type Repository struct {
	mu           sync.RWMutex
	items        map[string]*domain.StockItem
	reservations map[string]*domain.Reservation
	nextID       int
}

func NewRepository() *Repository {
	return &Repository{
		items:        make(map[string]*domain.StockItem),
		reservations: make(map[string]*domain.Reservation),
	}
}

func (r *Repository) NextReservationID(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	return fmt.Sprintf("res_%06d", r.nextID), nil
}

func (r *Repository) GetItem(ctx context.Context, sku string) (*domain.StockItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	item, ok := r.items[sku]
	if !ok {
		return nil, ports.ErrNotFound
	}
	copied := *item
	return &copied, nil
}

func (r *Repository) SaveItem(ctx context.Context, item *domain.StockItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	copied := *item
	r.items[item.SKU] = &copied
	return nil
}

func (r *Repository) GetReservation(ctx context.Context, id string) (*domain.Reservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	reservation, ok := r.reservations[id]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return cloneReservation(reservation), nil
}

func (r *Repository) SaveReservation(ctx context.Context, reservation *domain.Reservation) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.reservations[reservation.ID] = cloneReservation(reservation)
	return nil
}

func cloneReservation(reservation *domain.Reservation) *domain.Reservation {
	copied := *reservation
	copied.Items = make([]domain.ReservationItem, len(reservation.Items))
	copy(copied.Items, reservation.Items)
	return &copied
}
