package ports

import (
	"context"

	"github.com/elug3/dupli1/product/pkg/domain"
)

type ProductStore interface {
	SearchProducts(ctx context.Context, filter map[string]string) (results []domain.Product, total int, err error)
	ListProducts(ctx context.Context) ([]domain.Product, error)
	GetProduct(ctx context.Context, id string) (*domain.Product, error)
	GetActiveProduct(ctx context.Context, id string) (*domain.Product, error)
	CreateProduct(ctx context.Context, p domain.Product) (*domain.Product, error)
	UpdateProduct(ctx context.Context, p domain.Product) (*domain.Product, error)
	DeleteProduct(ctx context.Context, id string) error

	ListVariants(ctx context.Context, productID string) ([]domain.Variant, error)
	GetVariant(ctx context.Context, sku string) (*domain.Variant, error)
	GetVariantBySkuID(ctx context.Context, skuID string) (*domain.Variant, error)
	GetVariantsBySkuIDs(ctx context.Context, skuIDs []string) ([]domain.Variant, error)
	CreateVariant(ctx context.Context, v domain.Variant) (*domain.Variant, error)
	UpdateVariant(ctx context.Context, v domain.Variant) (*domain.Variant, error)
	DeleteVariant(ctx context.Context, sku string) error
}
