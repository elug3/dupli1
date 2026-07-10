package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/elug3/dupli1/cart/pkg/domain"
	"github.com/elug3/dupli1/cart/pkg/ports"
)

type Service struct {
	repo      ports.Repository
	product   ports.ProductClient
	inventory ports.InventoryClient
	now       func() time.Time
}

func New(repo ports.Repository, product ports.ProductClient, inventory ports.InventoryClient) *Service {
	return &Service{
		repo:      repo,
		product:   product,
		inventory: inventory,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

type ItemInput struct {
	SkuID    string
	SKU      string
	Quantity int
}

func (s *Service) GetCart(ctx context.Context, customerID string) (*domain.Cart, error) {
	customerID = strings.TrimSpace(customerID)
	if customerID == "" {
		return nil, domain.ErrInvalidCart
	}

	stored, updatedAt, err := s.repo.GetItems(ctx, customerID)
	if err != nil {
		return nil, err
	}
	return s.enrichCart(ctx, customerID, stored, updatedAt)
}

func (s *Service) ReplaceItems(ctx context.Context, customerID string, inputs []ItemInput) (*domain.Cart, error) {
	customerID = strings.TrimSpace(customerID)
	if customerID == "" {
		return nil, domain.ErrInvalidCart
	}

	stored := make([]domain.StoredItem, len(inputs))
	for i, input := range inputs {
		stored[i] = domain.StoredItem{
			SkuID:    strings.TrimSpace(input.SkuID),
			SKU:      domain.NormalizeSKU(input.SKU),
			Quantity: input.Quantity,
		}
	}
	if err := domain.ValidateStoredItems(stored); err != nil {
		return nil, err
	}
	for i, item := range stored {
		info, err := s.resolveVariant(ctx, item)
		if err != nil {
			return nil, err
		}
		stored[i].SkuID = info.SkuID
		stored[i].SKU = info.SKU
	}

	now := s.now()
	if err := s.repo.ReplaceItems(ctx, customerID, stored, now); err != nil {
		return nil, err
	}
	return s.enrichCart(ctx, customerID, stored, now)
}

func (s *Service) UpsertItem(ctx context.Context, customerID string, input ItemInput) (*domain.Cart, error) {
	customerID = strings.TrimSpace(customerID)
	if customerID == "" {
		return nil, domain.ErrInvalidCart
	}

	item := domain.StoredItem{
		SkuID:    strings.TrimSpace(input.SkuID),
		SKU:      domain.NormalizeSKU(input.SKU),
		Quantity: input.Quantity,
	}
	if err := domain.ValidateStoredItem(item); err != nil {
		return nil, err
	}
	info, err := s.resolveVariant(ctx, item)
	if err != nil {
		return nil, err
	}
	item.SkuID = info.SkuID
	item.SKU = info.SKU

	now := s.now()
	if err := s.repo.UpsertItem(ctx, customerID, item, now); err != nil {
		return nil, err
	}
	return s.GetCart(ctx, customerID)
}

func (s *Service) RemoveItem(ctx context.Context, customerID, sku string) (*domain.Cart, error) {
	customerID = strings.TrimSpace(customerID)
	sku = domain.NormalizeSKU(sku)
	if customerID == "" || sku == "" {
		return nil, domain.ErrInvalidCartItem
	}

	now := s.now()
	if err := s.repo.RemoveItem(ctx, customerID, sku, now); err != nil {
		return nil, err
	}
	return s.GetCart(ctx, customerID)
}

func (s *Service) RemoveItemBySkuID(ctx context.Context, customerID, skuID string) (*domain.Cart, error) {
	customerID = strings.TrimSpace(customerID)
	skuID = strings.TrimSpace(skuID)
	if customerID == "" || skuID == "" {
		return nil, domain.ErrInvalidCartItem
	}

	now := s.now()
	if err := s.repo.RemoveItemBySkuID(ctx, customerID, skuID, now); err != nil {
		return nil, err
	}
	return s.GetCart(ctx, customerID)
}

func (s *Service) ClearCart(ctx context.Context, customerID string) error {
	customerID = strings.TrimSpace(customerID)
	if customerID == "" {
		return domain.ErrInvalidCart
	}
	return s.repo.Clear(ctx, customerID)
}

// resolveVariant looks up a variant by whichever identifier the item carries,
// preferring SkuID when present. The returned VariantInfo always carries the
// canonical SkuID/SKU pair, regardless of which one was used to look it up.
func (s *Service) resolveVariant(ctx context.Context, item domain.StoredItem) (*ports.VariantInfo, error) {
	if s.product == nil {
		return nil, ports.ErrProductUnavailable
	}
	var (
		info *ports.VariantInfo
		err  error
	)
	if item.SkuID != "" {
		info, err = s.product.GetVariantBySkuID(ctx, item.SkuID)
	} else {
		info, err = s.product.GetVariant(ctx, item.SKU)
	}
	if err != nil {
		if errors.Is(err, ports.ErrVariantNotFound) {
			return nil, err
		}
		return nil, ports.ErrProductUnavailable
	}
	return info, nil
}

func (s *Service) enrichCart(ctx context.Context, customerID string, stored []domain.StoredItem, updatedAt time.Time) (*domain.Cart, error) {
	items := make([]domain.CartItem, 0, len(stored))
	var subtotal int64

	for _, item := range stored {
		enriched := domain.CartItem{
			SkuID:    item.SkuID,
			SKU:      item.SKU,
			Quantity: item.Quantity,
		}
		if info, err := s.resolveVariant(ctx, item); err == nil && info != nil {
			enriched.SkuID = info.SkuID
			enriched.SKU = info.SKU
			enriched.ProductID = info.ProductID
			enriched.Color = info.Color
			enriched.UnitPriceCents = info.UnitPriceCents
			enriched.ImageURL = info.ImageURL
		}
		if s.inventory != nil {
			qty, err := s.lookupAvailableQty(ctx, enriched.SkuID, item.SKU)
			if err == nil {
				enriched.AvailableQty = qty
			}
		}
		subtotal += int64(item.Quantity) * enriched.UnitPriceCents
		items = append(items, enriched)
	}

	return &domain.Cart{
		CustomerID:    customerID,
		Items:         items,
		SubtotalCents: subtotal,
		UpdatedAt:     updatedAt,
	}, nil
}

func (s *Service) lookupAvailableQty(ctx context.Context, skuID, sku string) (int, error) {
	if skuID != "" {
		return s.inventory.GetAvailableQtyBySkuID(ctx, skuID)
	}
	return s.inventory.GetAvailableQty(ctx, sku)
}
