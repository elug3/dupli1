package memory

import "github.com/schick/pkg/product/domain"

// ProductStore is an in-memory implementation of ports.ProductStore, for use in tests.
type ProductStore struct {
	Consultations []domain.Consultation
	Shoes         []domain.Shoes
	Outerwear     []domain.Outerwear
	Bottoms       []domain.Bottoms
	Bags          []domain.Bag
	Clocks        []domain.Clock
}

func NewProductStore() *ProductStore {
	return &ProductStore{}
}

func (s *ProductStore) SearchConsultations(_ map[string]string) ([]domain.Consultation, error) {
	return s.Consultations, nil
}

func (s *ProductStore) SearchShoes(_ map[string]string) ([]domain.Shoes, error) {
	return s.Shoes, nil
}

func (s *ProductStore) SearchOuterwear(_ map[string]string) ([]domain.Outerwear, error) {
	return s.Outerwear, nil
}

func (s *ProductStore) SearchBottoms(_ map[string]string) ([]domain.Bottoms, error) {
	return s.Bottoms, nil
}

func (s *ProductStore) SearchBags(_ map[string]string) ([]domain.Bag, error) {
	return s.Bags, nil
}

func (s *ProductStore) SearchClocks(_ map[string]string) ([]domain.Clock, error) {
	return s.Clocks, nil
}
