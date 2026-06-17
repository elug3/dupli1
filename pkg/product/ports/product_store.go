package ports

import "github.com/schick/pkg/product/domain"

type ProductStore interface {
	SearchBags(filter map[string]string) ([]domain.Bag, error)
	ListProducts() ([]domain.Product, error)
	GetProduct(id string) (*domain.Product, error)
	CreateProduct(p domain.Product) (*domain.Product, error)
	UpdateProduct(p domain.Product) (*domain.Product, error)
	DeleteProduct(id string) error
}
