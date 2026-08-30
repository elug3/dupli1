package ports

import (
	"context"

	"github.com/elug3/dupli1/product/pkg/domain"
)

// CatalogStore manages SKU segment master data (code → name dictionaries).
type CatalogStore interface {
	ListBrands(ctx context.Context) ([]domain.Brand, error)
	GetBrand(ctx context.Context, code string) (*domain.Brand, error)
	CreateBrand(ctx context.Context, b domain.Brand) (*domain.Brand, error)
	UpdateBrandName(ctx context.Context, code, name string) (*domain.Brand, error)
	DeleteBrand(ctx context.Context, code string) error

	ListStyles(ctx context.Context, brandCode string) ([]domain.Style, error)
	GetStyle(ctx context.Context, brandCode, code string) (*domain.Style, error)
	CreateStyle(ctx context.Context, s domain.Style) (*domain.Style, error)
	UpdateStyleName(ctx context.Context, brandCode, code, name string) (*domain.Style, error)
	DeleteStyle(ctx context.Context, brandCode, code string) error

	ListColors(ctx context.Context) ([]domain.Color, error)
	GetColor(ctx context.Context, code string) (*domain.Color, error)
	CreateColor(ctx context.Context, c domain.Color) (*domain.Color, error)
	UpdateColorName(ctx context.Context, code, name string) (*domain.Color, error)
	DeleteColor(ctx context.Context, code string) error

	ListSizes(ctx context.Context) ([]domain.Size, error)
	GetSize(ctx context.Context, code string) (*domain.Size, error)
	CreateSize(ctx context.Context, sz domain.Size) (*domain.Size, error)
	UpdateSizeName(ctx context.Context, code, name string) (*domain.Size, error)
	DeleteSize(ctx context.Context, code string) error

	ListEditions(ctx context.Context) ([]domain.Edition, error)
	GetEdition(ctx context.Context, code string) (*domain.Edition, error)
	CreateEdition(ctx context.Context, e domain.Edition) (*domain.Edition, error)
	UpdateEditionName(ctx context.Context, code, name string) (*domain.Edition, error)
	DeleteEdition(ctx context.Context, code string) error

	// Bag merchandising taxonomy (storefront filters).
	ListSubCategories(ctx context.Context) ([]domain.CatalogTerm, error)
	ListBagStyles(ctx context.Context) ([]domain.CatalogTerm, error)
	ListTargets(ctx context.Context) ([]domain.CatalogTerm, error)
}
