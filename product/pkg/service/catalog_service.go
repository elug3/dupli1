package service

import (
	"fmt"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

// CatalogService manages SKU master-data dictionaries (code → name).
type CatalogService struct {
	store ports.CatalogStore
}

func NewCatalogService(store ports.CatalogStore) *CatalogService {
	return &CatalogService{store: store}
}

func (s *CatalogService) ListBrands() ([]domain.Brand, error) {
	if s.store == nil {
		return nil, fmt.Errorf("catalog store not initialized")
	}
	return s.store.ListBrands()
}

func (s *CatalogService) CreateBrand(b domain.Brand) (*domain.Brand, error) {
	return s.store.CreateBrand(b)
}

func (s *CatalogService) UpdateBrandName(code, name string) (*domain.Brand, error) {
	return s.store.UpdateBrandName(code, name)
}

func (s *CatalogService) DeleteBrand(code string) error {
	return s.store.DeleteBrand(code)
}

func (s *CatalogService) ListStyles(brandCode string) ([]domain.Style, error) {
	return s.store.ListStyles(brandCode)
}

func (s *CatalogService) CreateStyle(st domain.Style) (*domain.Style, error) {
	return s.store.CreateStyle(st)
}

func (s *CatalogService) UpdateStyleName(brandCode, code, name string) (*domain.Style, error) {
	return s.store.UpdateStyleName(brandCode, code, name)
}

func (s *CatalogService) DeleteStyle(brandCode, code string) error {
	return s.store.DeleteStyle(brandCode, code)
}

func (s *CatalogService) ListColors() ([]domain.Color, error) {
	return s.store.ListColors()
}

func (s *CatalogService) CreateColor(c domain.Color) (*domain.Color, error) {
	return s.store.CreateColor(c)
}

func (s *CatalogService) UpdateColorName(code, name string) (*domain.Color, error) {
	return s.store.UpdateColorName(code, name)
}

func (s *CatalogService) DeleteColor(code string) error {
	return s.store.DeleteColor(code)
}

func (s *CatalogService) ListSizes() ([]domain.Size, error) {
	return s.store.ListSizes()
}

func (s *CatalogService) CreateSize(sz domain.Size) (*domain.Size, error) {
	return s.store.CreateSize(sz)
}

func (s *CatalogService) UpdateSizeName(code, name string) (*domain.Size, error) {
	return s.store.UpdateSizeName(code, name)
}

func (s *CatalogService) DeleteSize(code string) error {
	return s.store.DeleteSize(code)
}

func (s *CatalogService) ListEditions() ([]domain.Edition, error) {
	return s.store.ListEditions()
}

func (s *CatalogService) CreateEdition(e domain.Edition) (*domain.Edition, error) {
	return s.store.CreateEdition(e)
}

func (s *CatalogService) UpdateEditionName(code, name string) (*domain.Edition, error) {
	return s.store.UpdateEditionName(code, name)
}

func (s *CatalogService) DeleteEdition(code string) error {
	return s.store.DeleteEdition(code)
}

func (s *CatalogService) ListSubCategories() ([]domain.CatalogTerm, error) {
	if s.store == nil {
		return nil, fmt.Errorf("catalog store not initialized")
	}
	return s.store.ListSubCategories()
}

func (s *CatalogService) ListBagStyles() ([]domain.CatalogTerm, error) {
	if s.store == nil {
		return nil, fmt.Errorf("catalog store not initialized")
	}
	return s.store.ListBagStyles()
}

func (s *CatalogService) ListTargets() ([]domain.CatalogTerm, error) {
	if s.store == nil {
		return nil, fmt.Errorf("catalog store not initialized")
	}
	return s.store.ListTargets()
}

// MasterCatalog returns the bag merchandising taxonomy (subcategories, styles, targets).
func (s *CatalogService) MasterCatalog() (*domain.MasterCatalog, error) {
	subs, err := s.ListSubCategories()
	if err != nil {
		return nil, err
	}
	styles, err := s.ListBagStyles()
	if err != nil {
		return nil, err
	}
	targets, err := s.ListTargets()
	if err != nil {
		return nil, err
	}
	return &domain.MasterCatalog{
		SubCategories: subs,
		Styles:        styles,
		Targets:       targets,
	}, nil
}
