package service

import (
	"fmt"

	"github.com/schick/pkg/product/domain"
	"github.com/schick/pkg/product/ports"
)

type ProductSearchService struct {
	store ports.ProductStore
}

func NewProductSearchService(store ports.ProductStore) *ProductSearchService {
	return &ProductSearchService{store: store}
}

func (s *ProductSearchService) SearchConsultations(filter map[string]string) ([]domain.Consultation, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	return s.store.SearchConsultations(filter)
}

func (s *ProductSearchService) SearchShoes(filter map[string]string) ([]domain.Shoes, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	return s.store.SearchShoes(filter)
}

func (s *ProductSearchService) SearchOuterwear(filter map[string]string) ([]domain.Outerwear, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	return s.store.SearchOuterwear(filter)
}

func (s *ProductSearchService) SearchBottoms(filter map[string]string) ([]domain.Bottoms, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	return s.store.SearchBottoms(filter)
}

func (s *ProductSearchService) SearchBags(filter map[string]string) ([]domain.Bag, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	return s.store.SearchBags(filter)
}

func (s *ProductSearchService) SearchClocks(filter map[string]string) ([]domain.Clock, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	return s.store.SearchClocks(filter)
}

func (s *ProductSearchService) Search(category string, filter map[string]string) (interface{}, error) {
	switch category {
	case "consultations":
		return s.SearchConsultations(filter)
	case "shoes":
		return s.SearchShoes(filter)
	case "outerwear":
		return s.SearchOuterwear(filter)
	case "bottoms":
		return s.SearchBottoms(filter)
	case "bags":
		return s.SearchBags(filter)
	case "clocks":
		return s.SearchClocks(filter)
	default:
		return nil, fmt.Errorf("unknown category: %s", category)
	}
}
