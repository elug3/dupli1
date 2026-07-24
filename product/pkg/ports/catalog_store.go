package ports

import "github.com/elug3/dupli1/product/pkg/domain"

// CatalogStore manages SKU segment master data (code → name dictionaries).
type CatalogStore interface {
	ListBrands() ([]domain.Brand, error)
	GetBrand(code string) (*domain.Brand, error)
	CreateBrand(b domain.Brand) (*domain.Brand, error)
	UpdateBrandName(code, name string) (*domain.Brand, error)
	DeleteBrand(code string) error

	ListStyles(brandCode string) ([]domain.Style, error)
	GetStyle(brandCode, code string) (*domain.Style, error)
	CreateStyle(s domain.Style) (*domain.Style, error)
	UpdateStyleName(brandCode, code, name string) (*domain.Style, error)
	DeleteStyle(brandCode, code string) error

	ListColors() ([]domain.Color, error)
	GetColor(code string) (*domain.Color, error)
	CreateColor(c domain.Color) (*domain.Color, error)
	UpdateColorName(code, name string) (*domain.Color, error)
	DeleteColor(code string) error

	ListSizes() ([]domain.Size, error)
	GetSize(code string) (*domain.Size, error)
	CreateSize(sz domain.Size) (*domain.Size, error)
	UpdateSizeName(code, name string) (*domain.Size, error)
	DeleteSize(code string) error

	ListEditions() ([]domain.Edition, error)
	GetEdition(code string) (*domain.Edition, error)
	CreateEdition(e domain.Edition) (*domain.Edition, error)
	UpdateEditionName(code, name string) (*domain.Edition, error)
	DeleteEdition(code string) error

	// Bag merchandising taxonomy (storefront filters).
	ListSubCategories() ([]domain.CatalogTerm, error)
	ListBagStyles() ([]domain.CatalogTerm, error)
	ListTargets() ([]domain.CatalogTerm, error)
}
