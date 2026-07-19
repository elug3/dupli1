package ports

import "github.com/elug3/dupli1/product/pkg/domain"

type ProductStore interface {
	SearchProducts(filter map[string]string) (results []domain.Product, total int, err error)
	ListProducts() ([]domain.Product, error)
	GetProduct(id string) (*domain.Product, error)
	GetActiveProduct(id string) (*domain.Product, error)
	CreateProduct(p domain.Product) (*domain.Product, error)
	UpdateProduct(p domain.Product) (*domain.Product, error)
	DeleteProduct(id string) error

	ListVariants(productID string) ([]domain.Variant, error)
	GetVariant(sku string) (*domain.Variant, error)
	GetVariantBySkuID(skuID string) (*domain.Variant, error)
	GetVariantsBySkuIDs(skuIDs []string) ([]domain.Variant, error)
	CreateVariant(v domain.Variant) (*domain.Variant, error)
	UpdateVariant(v domain.Variant) (*domain.Variant, error)
	DeleteVariant(sku string) error
}
