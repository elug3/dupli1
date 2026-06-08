package ports

import "github.com/schick/pkg/product/domain"

type ProductStore interface {
	SearchBags(filter map[string]string) ([]domain.Bag, error)
	CreateProduct(p domain.Product) (*domain.Product, error)
}
