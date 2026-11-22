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

	// Base colors: the plain-color palette colorways point at.
	ListBaseColors(ctx context.Context) ([]domain.BaseColor, error)
	GetBaseColor(ctx context.Context, code string) (*domain.BaseColor, error)
	CreateBaseColor(ctx context.Context, b domain.BaseColor) (*domain.BaseColor, error)
	UpdateBaseColor(ctx context.Context, b domain.BaseColor) (*domain.BaseColor, error)
	// DeleteBaseColor fails with ErrMasterInUse while a colorway points at it.
	DeleteBaseColor(ctx context.Context, code string) error

	ListColors(ctx context.Context) ([]domain.Color, error)
	GetColor(ctx context.Context, code string) (*domain.Color, error)
	CreateColor(ctx context.Context, c domain.Color) (*domain.Color, error)
	// UpdateColor replaces the name, swatch hexes and base colors of an
	// existing color. An unknown base color is ErrMasterNotFound.
	UpdateColor(ctx context.Context, c domain.Color) (*domain.Color, error)
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

	TaxonomyStore
}

// TaxonomyReader lists the merchandising terms products are validated
// against (subcategories, bag styles, targets).
type TaxonomyReader interface {
	// ListTaxonomy returns one kind's terms ordered by sort order, then code.
	// Subcategories carry their category.
	ListTaxonomy(ctx context.Context, kind domain.TaxonomyKind) ([]domain.CatalogTerm, error)
}

// TaxonomyStore is the staff-edited merchandising taxonomy. Codes arrive
// normalized and validated by the service.
type TaxonomyStore interface {
	TaxonomyReader
	// CreateTaxonomyTerm fails with ErrMasterExists when the code is taken
	// (a subcategory code is unique across categories).
	CreateTaxonomyTerm(ctx context.Context, kind domain.TaxonomyKind, t domain.CatalogTerm) (*domain.CatalogTerm, error)
	// UpdateTaxonomyTerm fails with ErrMasterNotFound for an unknown code.
	UpdateTaxonomyTerm(ctx context.Context, kind domain.TaxonomyKind, code string, patch domain.CatalogTermPatch) (*domain.CatalogTerm, error)
	// DeleteTaxonomyTerm fails with ErrMasterInUse while a product carries
	// the code, and ErrMasterNotFound for an unknown one.
	DeleteTaxonomyTerm(ctx context.Context, kind domain.TaxonomyKind, code string) error
}
