package ports

import "github.com/elug3/dupli1/product/pkg/domain"

type ProductStore interface {
	SearchBags(filter map[string]string) ([]domain.Bag, error)
	ListProducts() ([]domain.Product, error)
	GetProduct(id string) (*domain.Product, error)
	GetActiveProduct(id string) (*domain.Product, error)
	CreateProduct(p domain.Product) (*domain.Product, error)
	UpdateProduct(p domain.Product) (*domain.Product, error)
	DeleteProduct(id string) error
}
