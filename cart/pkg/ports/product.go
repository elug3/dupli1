package ports

import (
	"context"
	"errors"
)

var (
	ErrVariantNotFound = errors.New("variant not found")
	ErrProductUnavailable = errors.New("product service unavailable")
)

type VariantInfo struct {
	SKU            string
	ProductID      string
	Color          string
	UnitPriceCents int64
	ImageURL       string
}

type ProductClient interface {
	GetVariant(ctx context.Context, sku string) (*VariantInfo, error)
}
