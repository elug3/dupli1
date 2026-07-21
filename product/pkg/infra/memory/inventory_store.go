package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

// InventoryStore is an in-memory implementation of ports.InventoryStore, for
// use in tests. Keyed by SkuID, same as the pg backend.
type InventoryStore struct {
	mu           sync.RWMutex
	items        map[string]*domain.StockItem
	reservations map[string]*domain.Reservation
	nextID       int
	// products is optional; when set, commit increments parent SoldCount
	// (mirrors pg FinalizeReservation joining product_variants → products).
	products *ProductStore
}

func NewInventoryStore() *InventoryStore {
	return &InventoryStore{
		items:        make(map[string]*domain.StockItem),
		reservations: make(map[string]*domain.Reservation),
	}
}

// WithProducts wires the catalog store so reservation commits bump SoldCount.
func (s *InventoryStore) WithProducts(products *ProductStore) *InventoryStore {
	s.products = products
	return s
}

func (s *InventoryStore) GetItem(ctx context.Context, skuID string) (*domain.StockItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	item, ok := s.items[skuID]
	if !ok {
		return nil, ports.ErrInventoryItemNotFound
	}
	copied := *item
	return &copied, nil
}

func (s *InventoryStore) SaveItem(ctx context.Context, item *domain.StockItem) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	copied := *item
	s.items[item.SkuID] = &copied
	return nil
}

func (s *InventoryStore) GetReservation(ctx context.Context, id string) (*domain.Reservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	reservation, ok := s.reservations[id]
	if !ok {
		return nil, ports.ErrInventoryItemNotFound
	}
	return cloneReservation(reservation), nil
}

func (s *InventoryStore) SaveReservation(ctx context.Context, reservation *domain.Reservation) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.reservations[reservation.ID] = cloneReservation(reservation)
	return nil
}

func (s *InventoryStore) CreateReservation(ctx context.Context, orderID string, items []domain.ReservationItem, now time.Time) (*domain.Reservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sorted := append([]domain.ReservationItem(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].SkuID < sorted[j].SkuID })

	for _, reservationItem := range sorted {
		item, ok := s.items[reservationItem.SkuID]
		if !ok {
			return nil, ports.ErrInventoryItemNotFound
		}
		if item.Available() < reservationItem.Quantity {
			return nil, ports.ErrInsufficientStock
		}
	}

	updatedItems := make(map[string]*domain.StockItem, len(sorted))
	for _, reservationItem := range sorted {
		item := updatedItems[reservationItem.SkuID]
		if item == nil {
			copied := *s.items[reservationItem.SkuID]
			item = &copied
		}
		item.Reserved += reservationItem.Quantity
		item.UpdatedAt = now
		updatedItems[reservationItem.SkuID] = item
	}
	for skuID, item := range updatedItems {
		s.items[skuID] = item
	}

	s.nextID++
	reservation := domain.NewReservation(fmt.Sprintf("res_%06d", s.nextID), orderID, items, now)
	s.reservations[reservation.ID] = cloneReservation(reservation)
	return cloneReservation(reservation), nil
}

func (s *InventoryStore) FinalizeReservation(ctx context.Context, id string, status domain.ReservationStatus, now time.Time) (*domain.Reservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	reservation, ok := s.reservations[id]
	if !ok {
		return nil, ports.ErrInventoryItemNotFound
	}
	if !reservation.IsActive() {
		return nil, ports.ErrReservationClosed
	}

	sorted := append([]domain.ReservationItem(nil), reservation.Items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].SkuID < sorted[j].SkuID })

	for _, reservationItem := range sorted {
		item, ok := s.items[reservationItem.SkuID]
		if !ok {
			return nil, ports.ErrInventoryItemNotFound
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

	if status == domain.ReservationCommitted {
		s.addSoldCounts(sorted)
	}

	reservation.Status = status
	reservation.UpdatedAt = now
	s.reservations[id] = cloneReservation(reservation)
	return cloneReservation(reservation), nil
}

func (s *InventoryStore) addSoldCounts(items []domain.ReservationItem) {
	if s.products == nil {
		return
	}
	deltas := make(map[string]int64, len(items))
	for _, item := range items {
		for _, v := range s.products.Variants {
			if v.SkuID == item.SkuID {
				deltas[v.ProductID] += int64(item.Quantity)
				break
			}
		}
	}
	for productID, qty := range deltas {
		for i := range s.products.Products {
			if s.products.Products[i].ID == productID {
				s.products.Products[i].SoldCount += qty
				break
			}
		}
	}
}

func cloneReservation(reservation *domain.Reservation) *domain.Reservation {
	copied := *reservation
	copied.Items = make([]domain.ReservationItem, len(reservation.Items))
	copy(copied.Items, reservation.Items)
	return &copied
}

// Ensure InventoryStore implements ports.InventoryStore at compile time.
var _ ports.InventoryStore = (*InventoryStore)(nil)
