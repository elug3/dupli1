package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

var (
	// ErrInvalidSKU covers both a blank sku/skuId reference and one that
	// doesn't resolve to any known variant — from inventory's point of view,
	// an unknown sku is equivalent to malformed input.
	ErrInvalidSKU        = errors.New("invalid sku")
	ErrInvalidQuantity   = errors.New("invalid quantity")
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrReservationClosed = errors.New("reservation is not active")
)

// SkuRef is a caller-supplied reference to a variant: either the canonical
// SkuID or the human sku string. SkuID wins when both are set.
type SkuRef struct {
	SkuID string
	SKU   string
}

// ReservationItemRef pairs a SkuRef with the quantity to reserve.
type ReservationItemRef struct {
	Ref      SkuRef
	Quantity int
}

// InventoryService is the product service's own stock/reservation service
// (merged in from the standalone inventory service). Every method resolves
// whichever identifier the caller sent to a canonical SkuID before touching
// the store, so old (sku) and new (skuId) callers can coexist indefinitely.
type InventoryService struct {
	store    ports.InventoryStore
	variants ports.VariantResolver
	now      func() time.Time
}

func NewInventoryService(store ports.InventoryStore, variants ports.VariantResolver) *InventoryService {
	return &InventoryService{
		store:    store,
		variants: variants,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *InventoryService) resolve(ref SkuRef) (skuID, sku string, err error) {
	if ref.SkuID != "" {
		v, err := s.variants.GetVariantBySkuID(ref.SkuID)
		if err != nil {
			return "", "", ErrInvalidSKU
		}
		return v.SkuID, v.SKU, nil
	}
	normalized := normalizeSKU(ref.SKU)
	if normalized == "" {
		return "", "", ErrInvalidSKU
	}
	v, err := s.variants.GetVariant(normalized)
	if err != nil {
		return "", "", ErrInvalidSKU
	}
	return v.SkuID, v.SKU, nil
}

func (s *InventoryService) UpsertItem(ctx context.Context, ref SkuRef, quantity int) (*domain.StockItem, error) {
	skuID, sku, err := s.resolve(ref)
	if err != nil {
		return nil, err
	}
	if quantity < 0 {
		return nil, ErrInvalidQuantity
	}

	now := s.now()
	item, err := s.store.GetItem(ctx, skuID)
	if err != nil && !errors.Is(err, ports.ErrInventoryItemNotFound) {
		return nil, err
	}
	if item == nil {
		item = &domain.StockItem{SkuID: skuID, SKU: sku}
	}
	if quantity < item.Reserved {
		return nil, ErrInsufficientStock
	}
	item.Quantity = quantity
	item.UpdatedAt = now

	if err := s.store.SaveItem(ctx, item); err != nil {
		return nil, err
	}
	return cloneItem(item), nil
}

func (s *InventoryService) GetItem(ctx context.Context, ref SkuRef) (*domain.StockItem, error) {
	skuID, _, err := s.resolve(ref)
	if err != nil {
		return nil, err
	}

	item, err := s.store.GetItem(ctx, skuID)
	if err != nil {
		return nil, err
	}
	return cloneItem(item), nil
}

func (s *InventoryService) AdjustStock(ctx context.Context, ref SkuRef, delta int) (*domain.StockItem, error) {
	skuID, sku, err := s.resolve(ref)
	if err != nil {
		return nil, err
	}

	item, err := s.store.GetItem(ctx, skuID)
	if err != nil {
		if !errors.Is(err, ports.ErrInventoryItemNotFound) {
			return nil, err
		}
		item = &domain.StockItem{SkuID: skuID, SKU: sku}
	}

	nextQuantity := item.Quantity + delta
	if nextQuantity < 0 || nextQuantity < item.Reserved {
		return nil, ErrInsufficientStock
	}

	item.Quantity = nextQuantity
	item.UpdatedAt = s.now()
	if err := s.store.SaveItem(ctx, item); err != nil {
		return nil, err
	}
	return cloneItem(item), nil
}

func (s *InventoryService) Reserve(ctx context.Context, orderID string, items []ReservationItemRef) (*domain.Reservation, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, fmt.Errorf("order id is required")
	}

	normalizedItems, err := s.resolveReservationItems(items)
	if err != nil {
		return nil, err
	}

	reservation, err := s.store.CreateReservation(ctx, orderID, normalizedItems, s.now())
	if err != nil {
		if errors.Is(err, ports.ErrInsufficientStock) {
			return nil, ErrInsufficientStock
		}
		if errors.Is(err, ports.ErrInventoryItemNotFound) {
			return nil, ports.ErrInventoryItemNotFound
		}
		return nil, err
	}
	return cloneReservation(reservation), nil
}

func (s *InventoryService) ReleaseReservation(ctx context.Context, id string) (*domain.Reservation, error) {
	return s.closeReservation(ctx, id, domain.ReservationReleased)
}

func (s *InventoryService) CommitReservation(ctx context.Context, id string) (*domain.Reservation, error) {
	return s.closeReservation(ctx, id, domain.ReservationCommitted)
}

func (s *InventoryService) closeReservation(ctx context.Context, id string, status domain.ReservationStatus) (*domain.Reservation, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("reservation id is required")
	}

	reservation, err := s.store.FinalizeReservation(ctx, id, status, s.now())
	if err != nil {
		if errors.Is(err, ports.ErrReservationClosed) {
			return nil, ErrReservationClosed
		}
		if errors.Is(err, ports.ErrInsufficientStock) {
			return nil, ErrInsufficientStock
		}
		return nil, err
	}
	return cloneReservation(reservation), nil
}

// resolveReservationItems resolves every item's SkuRef to a canonical SkuID
// and aggregates duplicate references (e.g. the same variant sent once by
// sku and once by skuId) into a single line, mirroring the original
// inventory service's dedupe-by-sku behavior.
func (s *InventoryService) resolveReservationItems(items []ReservationItemRef) ([]domain.ReservationItem, error) {
	if len(items) == 0 {
		return nil, ErrInvalidQuantity
	}

	type agg struct {
		sku      string
		quantity int
	}
	quantities := make(map[string]*agg, len(items))
	order := make([]string, 0, len(items))
	for _, item := range items {
		if item.Quantity <= 0 {
			return nil, ErrInvalidQuantity
		}
		skuID, sku, err := s.resolve(item.Ref)
		if err != nil {
			return nil, err
		}
		if existing, ok := quantities[skuID]; ok {
			existing.quantity += item.Quantity
		} else {
			quantities[skuID] = &agg{sku: sku, quantity: item.Quantity}
			order = append(order, skuID)
		}
	}

	normalized := make([]domain.ReservationItem, 0, len(order))
	for _, skuID := range order {
		a := quantities[skuID]
		normalized = append(normalized, domain.ReservationItem{SkuID: skuID, SKU: a.sku, Quantity: a.quantity})
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
