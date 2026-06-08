package product

import (
	"fmt"

	"github.com/schick/pkg/product/domain"
	"github.com/schick/pkg/product/ports"
)

type ProductSearchService struct {
	store ports.ProductStore
}

func NewProductSearchService(store ports.ProductStore) *ProductSearchService {
	return &ProductSearchService{
		store: store,
	}
}

func (s *ProductSearchService) SearchBags(filter map[string]string) ([]domain.Bag, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	return s.store.SearchBags(filter)
}

func (s *ProductSearchService) CreateProduct(p domain.Product) (*domain.Product, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	return s.store.CreateProduct(p)
}
