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
			SKU:      domain.NormalizeSKU(input.SKU),
			Quantity: input.Quantity,
		}
	}
	if err := domain.ValidateStoredItems(stored); err != nil {
		return nil, err
	}
	for _, item := range stored {
		if err := s.validateVariant(ctx, item.SKU); err != nil {
			return nil, err
		}
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
		SKU:      domain.NormalizeSKU(input.SKU),
		Quantity: input.Quantity,
	}
	if err := domain.ValidateStoredItem(item); err != nil {
		return nil, err
	}
	if err := s.validateVariant(ctx, item.SKU); err != nil {
		return nil, err
	}

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

func (s *Service) ClearCart(ctx context.Context, customerID string) error {
	customerID = strings.TrimSpace(customerID)
	if customerID == "" {
		return domain.ErrInvalidCart
	}
	return s.repo.Clear(ctx, customerID)
}

func (s *Service) validateVariant(ctx context.Context, sku string) error {
	if s.product == nil {
		return ports.ErrProductUnavailable
	}
	_, err := s.product.GetVariant(ctx, sku)
	if err != nil {
		if errors.Is(err, ports.ErrVariantNotFound) {
			return err
		}
		return ports.ErrProductUnavailable
	}
	return nil
}

func (s *Service) enrichCart(ctx context.Context, customerID string, stored []domain.StoredItem, updatedAt time.Time) (*domain.Cart, error) {
	items := make([]domain.CartItem, 0, len(stored))
	var subtotal int64

	for _, item := range stored {
		enriched := domain.CartItem{
			SKU:      item.SKU,
			Quantity: item.Quantity,
		}
		if s.product != nil {
			info, err := s.product.GetVariant(ctx, item.SKU)
			if err == nil && info != nil {
				enriched.ProductID = info.ProductID
				enriched.Color = info.Color
				enriched.UnitPriceCents = info.UnitPriceCents
				enriched.ImageURL = info.ImageURL
			}
		}
		if s.inventory != nil {
			if qty, err := s.inventory.GetAvailableQty(ctx, item.SKU); err == nil {
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
