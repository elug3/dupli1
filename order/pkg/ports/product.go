package ports

import (
	"context"
	"errors"
)

var (
	ErrVariantNotFound    = errors.New("variant not found")
	ErrProductUnavailable = errors.New("product service unavailable")
)

// VariantInfo is the catalog snapshot used to price order/checkout lines.
type VariantInfo struct {
	SkuID          string
	SKU            string
	ProductID      string
	UnitPriceCents int64  // whole KRW won (from product.price; not ×100)
	ProductName    string // display name from parent product
	ImageURL       string // first variant image URL, empty if none
}

// ProductClient looks up public variant data for server-side pricing.
type ProductClient interface {
	GetVariant(ctx context.Context, sku string) (*VariantInfo, error)
	GetVariantBySkuID(ctx context.Context, skuID string) (*VariantInfo, error)
}
