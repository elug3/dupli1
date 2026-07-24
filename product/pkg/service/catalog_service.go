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

func (s *CatalogService) ListSubcategories() ([]domain.Subcategory, error) {
	return s.store.ListSubcategories()
}

func (s *CatalogService) CreateSubcategory(item domain.Subcategory) (*domain.Subcategory, error) {
	return s.store.CreateSubcategory(item)
}

func (s *CatalogService) UpdateSubcategoryName(code, name string) (*domain.Subcategory, error) {
	return s.store.UpdateSubcategoryName(code, name)
}

func (s *CatalogService) DeleteSubcategory(code string) error {
	return s.store.DeleteSubcategory(code)
}

func (s *CatalogService) ListOccasions() ([]domain.Occasion, error) {
	return s.store.ListOccasions()
}

func (s *CatalogService) CreateOccasion(item domain.Occasion) (*domain.Occasion, error) {
	return s.store.CreateOccasion(item)
}

func (s *CatalogService) UpdateOccasionName(code, name string) (*domain.Occasion, error) {
	return s.store.UpdateOccasionName(code, name)
}

func (s *CatalogService) DeleteOccasion(code string) error {
	return s.store.DeleteOccasion(code)
}

func (s *CatalogService) ListTargets() ([]domain.Target, error) {
	return s.store.ListTargets()
}

func (s *CatalogService) CreateTarget(item domain.Target) (*domain.Target, error) {
	return s.store.CreateTarget(item)
}

func (s *CatalogService) UpdateTargetName(code, name string) (*domain.Target, error) {
	return s.store.UpdateTargetName(code, name)
}

func (s *CatalogService) DeleteTarget(code string) error {
	return s.store.DeleteTarget(code)
}
