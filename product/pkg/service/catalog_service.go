package service

import (
	"context"
	"fmt"
	"strings"

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
	if err := c.NormalizeSwatch(); err != nil {
		return nil, ports.Invalid(err.Error())
	}
	if err := c.NormalizeBases(); err != nil {
		return nil, ports.Invalid(err.Error())
	}
	return s.store.CreateColor(ctx, c)
}

// UpdateColor applies a partial update: omitted fields are kept, "" clears a
// hex or the base color.
func (s *CatalogService) UpdateColor(ctx context.Context, code string, patch domain.ColorPatch) (*domain.Color, error) {
	current, err := s.store.GetColor(ctx, code)
	if err != nil {
		return nil, err
	}
	next := patch.Apply(*current)
	if err := next.NormalizeSwatch(); err != nil {
		return nil, ports.Invalid(err.Error())
	}
	if err := next.NormalizeBases(); err != nil {
		return nil, ports.Invalid(err.Error())
	}
	return s.store.UpdateColor(ctx, next)
}

func (s *CatalogService) ListBaseColors(ctx context.Context) ([]domain.BaseColor, error) {
	return s.store.ListBaseColors(ctx)
}

func (s *CatalogService) CreateBaseColor(ctx context.Context, b domain.BaseColor) (*domain.BaseColor, error) {
	if err := b.Normalize(); err != nil {
		return nil, ports.Invalid(err.Error())
	}
	return s.store.CreateBaseColor(ctx, b)
}

// UpdateBaseColor renames a base color or changes its hex; omitted fields are kept.
func (s *CatalogService) UpdateBaseColor(ctx context.Context, code string, patch domain.BaseColorPatch) (*domain.BaseColor, error) {
	current, err := s.store.GetBaseColor(ctx, code)
	if err != nil {
		return nil, err
	}
	next := patch.Apply(*current)
	if err := next.Normalize(); err != nil {
		return nil, ports.Invalid(err.Error())
	}
	return s.store.UpdateBaseColor(ctx, next)
}

func (s *CatalogService) DeleteBaseColor(ctx context.Context, code string) error {
	return s.store.DeleteBaseColor(ctx, code)
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

// loadTaxonomy reads every merchandising term; a nil reader holds none.
func loadTaxonomy(ctx context.Context, r ports.TaxonomyReader) (domain.Taxonomy, error) {
	var tax domain.Taxonomy
	if r == nil {
		return tax, nil
	}
	var err error
	if tax.SubCategories, err = r.ListTaxonomy(ctx, domain.TaxonomySubCategory); err != nil {
		return tax, err
	}
	if tax.Styles, err = r.ListTaxonomy(ctx, domain.TaxonomyBagStyle); err != nil {
		return tax, err
	}
	if tax.Targets, err = r.ListTaxonomy(ctx, domain.TaxonomyTarget); err != nil {
		return tax, err
	}
	return tax, nil
}

// ListSubCategories returns one category's subcategories; "" means bags.
func (s *CatalogService) ListSubCategories(ctx context.Context, category string) ([]domain.CatalogTerm, error) {
	if s.store == nil {
		return nil, fmt.Errorf("catalog store not initialized")
	}
	c, err := lookupCategory(category)
	if err != nil {
		return nil, err
	}
	all, err := s.store.ListTaxonomy(ctx, domain.TaxonomySubCategory)
	if err != nil {
		return nil, err
	}
	return domain.Taxonomy{SubCategories: all}.SubCategoriesOf(c.Code), nil
}

// ListCategories returns every category products may carry, each with its
// subcategories.
func (s *CatalogService) ListCategories(ctx context.Context) ([]domain.Category, error) {
	if s.store == nil {
		return nil, fmt.Errorf("catalog store not initialized")
	}
	tax, err := loadTaxonomy(ctx, s.store)
	if err != nil {
		return nil, err
	}
	return domain.Categories(tax), nil
}

func lookupCategory(code string) (domain.Category, error) {
	if code == "" {
		code = domain.DefaultCategory
	}
	c, ok := domain.LookupCategory(code)
	if !ok {
		return domain.Category{}, ports.Invalid(fmt.Sprintf("invalid category %q", code))
	}
	return c, nil
}

func (s *CatalogService) ListBagStyles(ctx context.Context) ([]domain.CatalogTerm, error) {
	if s.store == nil {
		return nil, fmt.Errorf("catalog store not initialized")
	}
	return s.store.ListTaxonomy(ctx, domain.TaxonomyBagStyle)
}

func (s *CatalogService) ListTargets(ctx context.Context) ([]domain.CatalogTerm, error) {
	if s.store == nil {
		return nil, fmt.Errorf("catalog store not initialized")
	}
	return s.store.ListTaxonomy(ctx, domain.TaxonomyTarget)
}

// CreateTaxonomyTerm adds a subcategory, bag style or target. The code is
// normalized (lowercase) and must be a valid term code; a subcategory names
// one of the categories in code (blank means bags). A nil sortOrder puts the
// term after the last one in its list. A code already in use — for a
// subcategory, in any category — is ErrMasterExists.
func (s *CatalogService) CreateTaxonomyTerm(ctx context.Context, kind domain.TaxonomyKind, t domain.CatalogTerm, sortOrder *int) (*domain.CatalogTerm, error) {
	if !kind.Valid() {
		return nil, fmt.Errorf("unknown taxonomy kind %q", kind)
	}
	t.Code = domain.NormalizeTaxonomyCode(t.Code)
	t.Name = strings.TrimSpace(t.Name)
	if !domain.ValidTaxonomyCode(t.Code) {
		return nil, ports.Invalid(fmt.Sprintf("invalid code %q: lowercase letters, digits, '-' or '_', at most 32", t.Code))
	}
	if t.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	if sortOrder != nil && *sortOrder < 0 {
		return nil, ports.Invalid("sortOrder must not be negative")
	}
	if kind == domain.TaxonomySubCategory {
		c, err := lookupCategory(t.Category)
		if err != nil {
			return nil, err
		}
		t.Category = c.Code
	} else if strings.TrimSpace(t.Category) != "" {
		return nil, ports.Invalid("only a subcategory has a category")
	}
	if sortOrder != nil {
		t.SortOrder = *sortOrder
	} else {
		list, err := s.store.ListTaxonomy(ctx, kind)
		if err != nil {
			return nil, err
		}
		t.SortOrder = 0
		for _, other := range list {
			if other.Category == t.Category && other.SortOrder >= t.SortOrder {
				t.SortOrder = other.SortOrder + 1
			}
		}
	}
	return s.store.CreateTaxonomyTerm(ctx, kind, t)
}

// UpdateTaxonomyTerm renames a term and/or moves it in its list. Neither its
// code nor a subcategory's category changes, since products store the code.
func (s *CatalogService) UpdateTaxonomyTerm(ctx context.Context, kind domain.TaxonomyKind, code string, patch domain.CatalogTermPatch) (*domain.CatalogTerm, error) {
	if !kind.Valid() {
		return nil, fmt.Errorf("unknown taxonomy kind %q", kind)
	}
	if patch.Name == nil && patch.SortOrder == nil {
		return nil, ports.Invalid("nothing to change: send name and/or sortOrder")
	}
	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name == "" {
			return nil, ports.Invalid("name is required")
		}
		patch.Name = &name
	}
	if patch.SortOrder != nil && *patch.SortOrder < 0 {
		return nil, ports.Invalid("sortOrder must not be negative")
	}
	return s.store.UpdateTaxonomyTerm(ctx, kind, domain.NormalizeTaxonomyCode(code), patch)
}

// DeleteTaxonomyTerm removes a term no product carries (ErrMasterInUse
// otherwise).
func (s *CatalogService) DeleteTaxonomyTerm(ctx context.Context, kind domain.TaxonomyKind, code string) error {
	if !kind.Valid() {
		return fmt.Errorf("unknown taxonomy kind %q", kind)
	}
	return s.store.DeleteTaxonomyTerm(ctx, kind, domain.NormalizeTaxonomyCode(code))
}

// MasterCatalog returns one category's merchandising taxonomy: its
// subcategories plus the shared styles and targets. "" means bags.
func (s *CatalogService) MasterCatalog(ctx context.Context, category string) (*domain.MasterCatalog, error) {
	subs, err := s.ListSubCategories(ctx, category)
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
