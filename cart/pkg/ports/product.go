package ports

import (
	"context"
	"errors"
)

var (
	ErrVariantNotFound    = errors.New("variant not found")
	ErrProductUnavailable = errors.New("product service unavailable")
)

type VariantInfo struct {
	SkuID          string
	SKU            string
	ProductID      string
	Color          string
	UnitPriceCents int64 // whole KRW won (from product.price; not ×100)
	ImageURL       string
}

type ProductClient interface {
	GetVariant(ctx context.Context, sku string) (*VariantInfo, error)
	GetVariantBySkuID(ctx context.Context, skuID string) (*VariantInfo, error)
}
