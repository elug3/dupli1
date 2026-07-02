package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/elug3/dupli1/inventory/pkg/domain"
	"github.com/elug3/dupli1/inventory/pkg/ports"
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

func (r *Repository) CreateReservation(ctx context.Context, orderID string, items []domain.ReservationItem, now time.Time) (*domain.Reservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	sorted := append([]domain.ReservationItem(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].SKU < sorted[j].SKU })

	for _, reservationItem := range sorted {
		item, ok := r.items[reservationItem.SKU]
		if !ok {
			return nil, ports.ErrNotFound
		}
		if item.Available() < reservationItem.Quantity {
			return nil, ports.ErrInsufficientStock
		}
	}

	updatedItems := make(map[string]*domain.StockItem, len(sorted))
	for _, reservationItem := range sorted {
		item := updatedItems[reservationItem.SKU]
		if item == nil {
			copied := *r.items[reservationItem.SKU]
			item = &copied
		}
		item.Reserved += reservationItem.Quantity
		item.UpdatedAt = now
		updatedItems[reservationItem.SKU] = item
	}
	for sku, item := range updatedItems {
		r.items[sku] = item
	}

	r.nextID++
	reservation := domain.NewReservation(fmt.Sprintf("res_%06d", r.nextID), orderID, items, now)
	r.reservations[reservation.ID] = cloneReservation(reservation)
	return cloneReservation(reservation), nil
}

func (r *Repository) FinalizeReservation(ctx context.Context, id string, status domain.ReservationStatus, now time.Time) (*domain.Reservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	reservation, ok := r.reservations[id]
	if !ok {
		return nil, ports.ErrNotFound
	}
	if !reservation.IsActive() {
		return nil, ports.ErrReservationClosed
	}

	sorted := append([]domain.ReservationItem(nil), reservation.Items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].SKU < sorted[j].SKU })

	for _, reservationItem := range sorted {
		item, ok := r.items[reservationItem.SKU]
		if !ok {
			return nil, ports.ErrNotFound
		}
		if item.Reserved < reservationItem.Quantity {
			return nil, ports.ErrInsufficientStock
		}
		item.Reserved -= reservationItem.Quantity
		if status == domain.ReservationCommitted {
			if item.Quantity < reservationItem.Quantity {
				return nil, ports.ErrInsufficientStock
			}
			item.Quantity -= reservationItem.Quantity
		}
		item.UpdatedAt = now
	}

	reservation.Status = status
	reservation.UpdatedAt = now
	r.reservations[id] = cloneReservation(reservation)
	return cloneReservation(reservation), nil
}

func cloneReservation(reservation *domain.Reservation) *domain.Reservation {
	copied := *reservation
	copied.Items = make([]domain.ReservationItem, len(reservation.Items))
	copy(copied.Items, reservation.Items)
	return &copied
}
