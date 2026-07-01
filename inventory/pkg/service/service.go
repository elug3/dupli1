package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/elug3/dupli1/inventory/pkg/domain"
	"github.com/elug3/dupli1/inventory/pkg/ports"
)

var (
	ErrInvalidSKU        = errors.New("invalid sku")
	ErrInvalidQuantity   = errors.New("invalid quantity")
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrReservationClosed = errors.New("reservation is not active")
)

type Service struct {
	repo ports.Repository
	now  func() time.Time
}

func New(repo ports.Repository) *Service {
	return &Service{
		repo: repo,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *Service) UpsertItem(ctx context.Context, sku string, quantity int) (*domain.StockItem, error) {
	sku = normalizeSKU(sku)
	if sku == "" {
		return nil, ErrInvalidSKU
	}
	if quantity < 0 {
		return nil, ErrInvalidQuantity
	}

	now := s.now()
	item, err := s.repo.GetItem(ctx, sku)
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return nil, err
	}
	if item == nil {
		item = &domain.StockItem{SKU: sku}
	}
	if quantity < item.Reserved {
		return nil, ErrInsufficientStock
	}
	item.Quantity = quantity
	item.UpdatedAt = now

	if err := s.repo.SaveItem(ctx, item); err != nil {
		return nil, err
	}
	return cloneItem(item), nil
}

func (s *Service) GetItem(ctx context.Context, sku string) (*domain.StockItem, error) {
	sku = normalizeSKU(sku)
	if sku == "" {
		return nil, ErrInvalidSKU
	}

	item, err := s.repo.GetItem(ctx, sku)
	if err != nil {
		return nil, err
	}
	return cloneItem(item), nil
}

func (s *Service) AdjustStock(ctx context.Context, sku string, delta int) (*domain.StockItem, error) {
	sku = normalizeSKU(sku)
	if sku == "" {
		return nil, ErrInvalidSKU
	}

	item, err := s.repo.GetItem(ctx, sku)
	if err != nil {
		if !errors.Is(err, ports.ErrNotFound) {
			return nil, err
		}
		item = &domain.StockItem{SKU: sku}
	}

	nextQuantity := item.Quantity + delta
	if nextQuantity < 0 || nextQuantity < item.Reserved {
		return nil, ErrInsufficientStock
	}

	item.Quantity = nextQuantity
	item.UpdatedAt = s.now()
	if err := s.repo.SaveItem(ctx, item); err != nil {
		return nil, err
	}
	return cloneItem(item), nil
}

func (s *Service) Reserve(ctx context.Context, orderID string, items []domain.ReservationItem) (*domain.Reservation, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, fmt.Errorf("order id is required")
	}

	normalizedItems, err := normalizeReservationItems(items)
	if err != nil {
		return nil, err
	}

	stockItems := make([]*domain.StockItem, 0, len(normalizedItems))
	for _, reservationItem := range normalizedItems {
		item, err := s.repo.GetItem(ctx, reservationItem.SKU)
		if err != nil {
			return nil, err
		}
		if item.Available() < reservationItem.Quantity {
			return nil, ErrInsufficientStock
		}
		copied := cloneItem(item)
		copied.Reserved += reservationItem.Quantity
		copied.UpdatedAt = s.now()
		stockItems = append(stockItems, copied)
	}

	reservationID, err := s.repo.NextReservationID(ctx)
	if err != nil {
		return nil, err
	}
	reservation := domain.NewReservation(reservationID, orderID, normalizedItems, s.now())

	for _, item := range stockItems {
		if err := s.repo.SaveItem(ctx, item); err != nil {
			return nil, err
		}
	}
	if err := s.repo.SaveReservation(ctx, reservation); err != nil {
		return nil, err
	}

	return cloneReservation(reservation), nil
}

func (s *Service) ReleaseReservation(ctx context.Context, id string) (*domain.Reservation, error) {
	return s.closeReservation(ctx, id, domain.ReservationReleased)
}

func (s *Service) CommitReservation(ctx context.Context, id string) (*domain.Reservation, error) {
	return s.closeReservation(ctx, id, domain.ReservationCommitted)
}

func (s *Service) closeReservation(ctx context.Context, id string, status domain.ReservationStatus) (*domain.Reservation, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("reservation id is required")
	}

	reservation, err := s.repo.GetReservation(ctx, id)
	if err != nil {
		return nil, err
	}
	if !reservation.IsActive() {
		return nil, ErrReservationClosed
	}

	now := s.now()
	for _, reservationItem := range reservation.Items {
		item, err := s.repo.GetItem(ctx, reservationItem.SKU)
		if err != nil {
			return nil, err
		}
		if item.Reserved < reservationItem.Quantity {
			return nil, ErrInsufficientStock
		}
		item.Reserved -= reservationItem.Quantity
		if status == domain.ReservationCommitted {
			if item.Quantity < reservationItem.Quantity {
				return nil, ErrInsufficientStock
			}
			item.Quantity -= reservationItem.Quantity
		}
		item.UpdatedAt = now
		if err := s.repo.SaveItem(ctx, item); err != nil {
			return nil, err
		}
	}

	reservation.Status = status
	reservation.UpdatedAt = now
	if err := s.repo.SaveReservation(ctx, reservation); err != nil {
		return nil, err
	}
	return cloneReservation(reservation), nil
}

func normalizeReservationItems(items []domain.ReservationItem) ([]domain.ReservationItem, error) {
	if len(items) == 0 {
		return nil, ErrInvalidQuantity
	}

	quantities := make(map[string]int, len(items))
	for _, item := range items {
		sku := normalizeSKU(item.SKU)
		if sku == "" {
			return nil, ErrInvalidSKU
		}
		if item.Quantity <= 0 {
			return nil, ErrInvalidQuantity
		}
		quantities[sku] += item.Quantity
	}

	normalized := make([]domain.ReservationItem, 0, len(quantities))
	for sku, quantity := range quantities {
		normalized = append(normalized, domain.ReservationItem{SKU: sku, Quantity: quantity})
	}
	return normalized, nil
}

func normalizeSKU(sku string) string {
	return strings.ToUpper(strings.TrimSpace(sku))
}

func cloneItem(item *domain.StockItem) *domain.StockItem {
	if item == nil {
		return nil
	}
	copied := *item
	return &copied
}

func cloneReservation(reservation *domain.Reservation) *domain.Reservation {
	if reservation == nil {
		return nil
	}
	copied := *reservation
	copied.Items = make([]domain.ReservationItem, len(reservation.Items))
	copy(copied.Items, reservation.Items)
	return &copied
}
