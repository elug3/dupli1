package service

import (
	"context"
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

func (s *CatalogService) ListBrands(ctx context.Context) ([]domain.Brand, error) {
	if s.store == nil {
		return nil, fmt.Errorf("catalog store not initialized")
	}
	return s.store.ListBrands(ctx)
}

func (s *CatalogService) CreateBrand(ctx context.Context, b domain.Brand) (*domain.Brand, error) {
	return s.store.CreateBrand(ctx, b)
}

func (s *CatalogService) UpdateBrandName(ctx context.Context, code, name string) (*domain.Brand, error) {
	return s.store.UpdateBrandName(ctx, code, name)
}

func (s *CatalogService) DeleteBrand(ctx context.Context, code string) error {
	return s.store.DeleteBrand(ctx, code)
}

func (s *CatalogService) ListStyles(ctx context.Context, brandCode string) ([]domain.Style, error) {
	return s.store.ListStyles(ctx, brandCode)
}

func (s *CatalogService) CreateStyle(ctx context.Context, st domain.Style) (*domain.Style, error) {
	return s.store.CreateStyle(ctx, st)
}

func (s *CatalogService) UpdateStyleName(ctx context.Context, brandCode, code, name string) (*domain.Style, error) {
	return s.store.UpdateStyleName(ctx, brandCode, code, name)
}

func (s *CatalogService) DeleteStyle(ctx context.Context, brandCode, code string) error {
	return s.store.DeleteStyle(ctx, brandCode, code)
}

func (s *CatalogService) ListColors(ctx context.Context) ([]domain.Color, error) {
	return s.store.ListColors(ctx)
}

func (s *CatalogService) CreateColor(ctx context.Context, c domain.Color) (*domain.Color, error) {
	return s.store.CreateColor(ctx, c)
}

func (s *CatalogService) UpdateColorName(ctx context.Context, code, name string) (*domain.Color, error) {
	return s.store.UpdateColorName(ctx, code, name)
}

func (s *CatalogService) DeleteColor(ctx context.Context, code string) error {
	return s.store.DeleteColor(ctx, code)
}

func (s *CatalogService) ListSizes(ctx context.Context) ([]domain.Size, error) {
	return s.store.ListSizes(ctx)
}

func (s *CatalogService) CreateSize(ctx context.Context, sz domain.Size) (*domain.Size, error) {
	return s.store.CreateSize(ctx, sz)
}

func (s *CatalogService) UpdateSizeName(ctx context.Context, code, name string) (*domain.Size, error) {
	return s.store.UpdateSizeName(ctx, code, name)
}

func (s *CatalogService) DeleteSize(ctx context.Context, code string) error {
	return s.store.DeleteSize(ctx, code)
}

func (s *CatalogService) ListEditions(ctx context.Context) ([]domain.Edition, error) {
	return s.store.ListEditions(ctx)
}

func (s *CatalogService) CreateEdition(ctx context.Context, e domain.Edition) (*domain.Edition, error) {
	return s.store.CreateEdition(ctx, e)
}

func (s *CatalogService) UpdateEditionName(ctx context.Context, code, name string) (*domain.Edition, error) {
	return s.store.UpdateEditionName(ctx, code, name)
}

func (s *CatalogService) DeleteEdition(ctx context.Context, code string) error {
	return s.store.DeleteEdition(ctx, code)
}

func (s *CatalogService) ListSubCategories(ctx context.Context) ([]domain.CatalogTerm, error) {
	if s.store == nil {
		return nil, fmt.Errorf("catalog store not initialized")
	}
	return s.store.ListSubCategories(ctx)
}

func (s *CatalogService) ListBagStyles(ctx context.Context) ([]domain.CatalogTerm, error) {
	if s.store == nil {
		return nil, fmt.Errorf("catalog store not initialized")
	}
	return s.store.ListBagStyles(ctx)
}

func (s *CatalogService) ListTargets(ctx context.Context) ([]domain.CatalogTerm, error) {
	if s.store == nil {
		return nil, fmt.Errorf("catalog store not initialized")
	}
	return s.store.ListTargets(ctx)
}

// MasterCatalog returns the bag merchandising taxonomy (subcategories, styles, targets).
func (s *CatalogService) MasterCatalog(ctx context.Context) (*domain.MasterCatalog, error) {
	subs, err := s.ListSubCategories(ctx)
	if err != nil {
		return nil, err
	}
	styles, err := s.ListBagStyles(ctx)
	if err != nil {
		return nil, err
	}
	targets, err := s.ListTargets(ctx)
	if err != nil {
		return nil, err
	}
	return &domain.MasterCatalog{
		SubCategories: subs,
		Styles:        styles,
		Targets:       targets,
	}, nil
}
