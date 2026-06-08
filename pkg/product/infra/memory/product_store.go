package memory

import (
	"github.com/google/uuid"
	"github.com/schick/pkg/product/domain"
)

// ProductStore is an in-memory implementation of ports.ProductStore, for use in tests.
type ProductStore struct {
	Bags []domain.Bag
}

func NewProductStore() *ProductStore {
	return &ProductStore{}
}

func (s *ProductStore) SearchBags(_ map[string]string) ([]domain.Bag, error) {
	return s.Bags, nil
}

func (s *ProductStore) CreateProduct(p domain.Product) (*domain.Product, error) {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	return &p, nil
}
