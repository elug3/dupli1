package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

// CatalogStore is an in-memory ports.CatalogStore for tests.
type CatalogStore struct {
	Brands        map[string]domain.Brand
	Styles        map[string]domain.Style // key: brandCode+"|"+code
	Colors        map[string]domain.Color
	BaseColors    map[string]domain.BaseColor
	Sizes         map[string]domain.Size
	Editions      map[string]domain.Edition
	SubCategories map[string]domain.CatalogTerm
	BagStyles     map[string]domain.CatalogTerm
	Targets       map[string]domain.CatalogTerm

	// Optional back-refs for delete-in-use checks (set by tests or product store).
	ProductBrandStyles  map[string]string // productID -> brandCode+"|"+styleCode
	VariantColorCodes   map[string]string // sku -> colorCode
	VariantSizeCodes    map[string]string // sku -> sizeCode
	VariantEditionCodes map[string]string
}

func NewCatalogStore() *CatalogStore {
	s := &CatalogStore{
		Brands:              map[string]domain.Brand{},
		Styles:              map[string]domain.Style{},
		Colors:              map[string]domain.Color{},
		BaseColors:          map[string]domain.BaseColor{},
		Sizes:               map[string]domain.Size{},
		Editions:            map[string]domain.Edition{},
		SubCategories:       map[string]domain.CatalogTerm{},
		BagStyles:           map[string]domain.CatalogTerm{},
		Targets:             map[string]domain.CatalogTerm{},
		ProductBrandStyles:  map[string]string{},
		VariantColorCodes:   map[string]string{},
		VariantSizeCodes:    map[string]string{},
		VariantEditionCodes: map[string]string{},
	}
	for _, b := range domain.SeedBrands {
		s.Brands[b.Code] = b
	}
	for _, b := range domain.SeedBaseColors() {
		s.BaseColors[b.Code] = b
	}
	for _, c := range domain.SeedColors {
		c.BaseCode = c.Code // each seed colorway is its own base color
		s.Colors[c.Code] = c
	}
	for _, sz := range domain.SizeMasterSeeds() {
		s.Sizes[sz.Code] = sz
	}
	for _, e := range domain.SeedEditions {
		s.Editions[e.Code] = e
	}
	for _, t := range domain.SeedSubCategories {
		s.SubCategories[t.Code] = t
	}
	for _, t := range domain.SeedBagStyles {
		s.BagStyles[t.Code] = t
	}
	for _, t := range domain.SeedTargets {
		s.Targets[t.Code] = t
	}
	return s
}

// codeByName mirrors the Postgres store: the master code a display name (or
// code) refers to — an exact code first, then a case-insensitive name, the
// smallest code winning a tie. "" when nothing matches.
func codeByName[T any](entries map[string]T, name string, nameOf func(T) string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	if _, ok := entries[domain.NormalizeCode(trimmed)]; ok {
		return domain.NormalizeCode(trimmed)
	}
	best := ""
	for code, e := range entries {
		if strings.EqualFold(nameOf(e), trimmed) && (best == "" || code < best) {
			best = code
		}
	}
	return best
}

// BrandCodeByName resolves a brand display name against the brand master.
func (s *CatalogStore) BrandCodeByName(name string) string {
	return codeByName(s.Brands, name, func(b domain.Brand) string { return b.Name })
}

// ColorCodeByName resolves a color display name against the color master.
func (s *CatalogStore) ColorCodeByName(name string) string {
	return codeByName(s.Colors, name, func(c domain.Color) string { return c.Name })
}

// SizeCodeByName resolves a size display name against the size master.
func (s *CatalogStore) SizeCodeByName(name string) string {
	return codeByName(s.Sizes, name, func(sz domain.Size) string { return sz.Name })
}

func styleKey(brand, code string) string {
	return brand + "|" + code
}

func (s *CatalogStore) ListBrands(ctx context.Context) ([]domain.Brand, error) {
	_ = ctx
	out := make([]domain.Brand, 0, len(s.Brands))
	for _, b := range s.Brands {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func (s *CatalogStore) GetBrand(ctx context.Context, code string) (*domain.Brand, error) {
	_ = ctx
	b, ok := s.Brands[domain.NormalizeCode(code)]
	if !ok {
		return nil, domain.ErrMasterNotFound
	}
	return &b, nil
}

func (s *CatalogStore) CreateBrand(ctx context.Context, b domain.Brand) (*domain.Brand, error) {
	_ = ctx
	b.Code = domain.NormalizeCode(b.Code)
	b.Name = strings.TrimSpace(b.Name)
	if !domain.ValidBrandCode(b.Code) {
		return nil, ports.Invalid(fmt.Sprintf("invalid brand code %q", b.Code))
	}
	if b.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	if _, ok := s.Brands[b.Code]; ok {
		return nil, domain.ErrMasterExists
	}
	s.Brands[b.Code] = b
	return &b, nil
}

func (s *CatalogStore) UpdateBrandName(ctx context.Context, code, name string) (*domain.Brand, error) {
	_ = ctx
	code = domain.NormalizeCode(code)
	name = strings.TrimSpace(name)
	b, ok := s.Brands[code]
	if !ok {
		return nil, domain.ErrMasterNotFound
	}
	if name == "" {
		return nil, ports.Invalid("name is required")
	}
	b.Name = name
	s.Brands[code] = b
	return &b, nil
}

func (s *CatalogStore) DeleteBrand(ctx context.Context, code string) error {
	_ = ctx
	code = domain.NormalizeCode(code)
	if _, ok := s.Brands[code]; !ok {
		return domain.ErrMasterNotFound
	}
	for k := range s.Styles {
		if strings.HasPrefix(k, code+"|") {
			return domain.ErrMasterInUse
		}
	}
	for _, ref := range s.ProductBrandStyles {
		if strings.HasPrefix(ref, code+"|") {
			return domain.ErrMasterInUse
		}
	}
	delete(s.Brands, code)
	return nil
}

func (s *CatalogStore) ListStyles(ctx context.Context, brandCode string) ([]domain.Style, error) {
	_ = ctx
	brandCode = domain.NormalizeCode(brandCode)
	var out []domain.Style
	for _, st := range s.Styles {
		if st.BrandCode == brandCode {
			out = append(out, st)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func (s *CatalogStore) GetStyle(ctx context.Context, brandCode, code string) (*domain.Style, error) {
	_ = ctx
	st, ok := s.Styles[styleKey(domain.NormalizeCode(brandCode), domain.NormalizeCode(code))]
	if !ok {
		return nil, domain.ErrMasterNotFound
	}
	return &st, nil
}

func (s *CatalogStore) CreateStyle(ctx context.Context, st domain.Style) (*domain.Style, error) {
	_ = ctx
	st.BrandCode = domain.NormalizeCode(st.BrandCode)
	st.Code = domain.NormalizeCode(st.Code)
	st.Name = strings.TrimSpace(st.Name)
	if !domain.ValidBrandCode(st.BrandCode) {
		return nil, ports.Invalid(fmt.Sprintf("invalid brand code %q", st.BrandCode))
	}
	if !domain.ValidSegmentCode(st.Code) {
		return nil, ports.Invalid(fmt.Sprintf("invalid style code %q", st.Code))
	}
	if st.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	if _, ok := s.Brands[st.BrandCode]; !ok {
		return nil, fmt.Errorf("%w: brand %s", domain.ErrMasterNotFound, st.BrandCode)
	}
	key := styleKey(st.BrandCode, st.Code)
	if _, ok := s.Styles[key]; ok {
		return nil, domain.ErrMasterExists
	}
	s.Styles[key] = st
	return &st, nil
}

func (s *CatalogStore) UpdateStyleName(ctx context.Context, brandCode, code, name string) (*domain.Style, error) {
	_ = ctx
	key := styleKey(domain.NormalizeCode(brandCode), domain.NormalizeCode(code))
	st, ok := s.Styles[key]
	if !ok {
		return nil, domain.ErrMasterNotFound
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ports.Invalid("name is required")
	}
	st.Name = name
	s.Styles[key] = st
	return &st, nil
}

func (s *CatalogStore) DeleteStyle(ctx context.Context, brandCode, code string) error {
	_ = ctx
	key := styleKey(domain.NormalizeCode(brandCode), domain.NormalizeCode(code))
	if _, ok := s.Styles[key]; !ok {
		return domain.ErrMasterNotFound
	}
	for _, ref := range s.ProductBrandStyles {
		if ref == key {
			return domain.ErrMasterInUse
		}
	}
	delete(s.Styles, key)
	return nil
}

func (s *CatalogStore) ListColors(ctx context.Context) ([]domain.Color, error) {
	_ = ctx
	out := make([]domain.Color, 0, len(s.Colors))
	for _, c := range s.Colors {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func (s *CatalogStore) GetColor(ctx context.Context, code string) (*domain.Color, error) {
	_ = ctx
	c, ok := s.Colors[domain.NormalizeCode(code)]
	if !ok {
		return nil, domain.ErrMasterNotFound
	}
	return &c, nil
}

func (s *CatalogStore) CreateColor(ctx context.Context, c domain.Color) (*domain.Color, error) {
	_ = ctx
	c.Code = domain.NormalizeCode(c.Code)
	c.Name = strings.TrimSpace(c.Name)
	if !domain.ValidSegmentCode(c.Code) {
		return nil, ports.Invalid(fmt.Sprintf("invalid color code %q", c.Code))
	}
	if c.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	if _, ok := s.Colors[c.Code]; ok {
		return nil, domain.ErrMasterExists
	}
	if err := s.requireBases(c); err != nil {
		return nil, err
	}
	s.Colors[c.Code] = c
	return &c, nil
}

func (s *CatalogStore) UpdateColor(ctx context.Context, c domain.Color) (*domain.Color, error) {
	_ = ctx
	c.Code = domain.NormalizeCode(c.Code)
	if _, ok := s.Colors[c.Code]; !ok {
		return nil, domain.ErrMasterNotFound
	}
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	if err := s.requireBases(c); err != nil {
		return nil, err
	}
	s.Colors[c.Code] = c
	return &c, nil
}

// requireBases mirrors the Postgres foreign keys from colors to base_colors.
func (s *CatalogStore) requireBases(c domain.Color) error {
	for _, code := range []string{c.BaseCode, c.Base2Code} {
		if code == "" {
			continue
		}
		if _, ok := s.BaseColors[code]; !ok {
			return fmt.Errorf("%w: base color %s", domain.ErrMasterNotFound, code)
		}
	}
	return nil
}

func (s *CatalogStore) ListBaseColors(ctx context.Context) ([]domain.BaseColor, error) {
	_ = ctx
	out := make([]domain.BaseColor, 0, len(s.BaseColors))
	for _, b := range s.BaseColors {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func (s *CatalogStore) GetBaseColor(ctx context.Context, code string) (*domain.BaseColor, error) {
	_ = ctx
	b, ok := s.BaseColors[domain.NormalizeCode(code)]
	if !ok {
		return nil, domain.ErrMasterNotFound
	}
	return &b, nil
}

func (s *CatalogStore) CreateBaseColor(ctx context.Context, b domain.BaseColor) (*domain.BaseColor, error) {
	_ = ctx
	if _, ok := s.BaseColors[b.Code]; ok {
		return nil, domain.ErrMasterExists
	}
	s.BaseColors[b.Code] = b
	return &b, nil
}

func (s *CatalogStore) UpdateBaseColor(ctx context.Context, b domain.BaseColor) (*domain.BaseColor, error) {
	_ = ctx
	if _, ok := s.BaseColors[b.Code]; !ok {
		return nil, domain.ErrMasterNotFound
	}
	s.BaseColors[b.Code] = b
	return &b, nil
}

func (s *CatalogStore) DeleteBaseColor(ctx context.Context, code string) error {
	_ = ctx
	code = domain.NormalizeCode(code)
	if _, ok := s.BaseColors[code]; !ok {
		return domain.ErrMasterNotFound
	}
	for _, c := range s.Colors {
		if c.BaseCode == code || c.Base2Code == code {
			return domain.ErrMasterInUse
		}
	}
	delete(s.BaseColors, code)
	return nil
}

func (s *CatalogStore) DeleteColor(ctx context.Context, code string) error {
	_ = ctx
	code = domain.NormalizeCode(code)
	if _, ok := s.Colors[code]; !ok {
		return domain.ErrMasterNotFound
	}
	for _, c := range s.VariantColorCodes {
		if c == code {
			return domain.ErrMasterInUse
		}
	}
	delete(s.Colors, code)
	return nil
}

func (s *CatalogStore) ListSizes(ctx context.Context) ([]domain.Size, error) {
	_ = ctx
	out := make([]domain.Size, 0, len(s.Sizes))
	for _, sz := range s.Sizes {
		out = append(out, sz)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func (s *CatalogStore) GetSize(ctx context.Context, code string) (*domain.Size, error) {
	_ = ctx
	sz, ok := s.Sizes[domain.NormalizeCode(code)]
	if !ok {
		return nil, domain.ErrMasterNotFound
	}
	return &sz, nil
}

func (s *CatalogStore) CreateSize(ctx context.Context, sz domain.Size) (*domain.Size, error) {
	_ = ctx
	sz.Code = domain.NormalizeCode(sz.Code)
	sz.Name = strings.TrimSpace(sz.Name)
	if !domain.ValidSegmentCode(sz.Code) {
		return nil, ports.Invalid(fmt.Sprintf("invalid size code %q", sz.Code))
	}
	if sz.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	if _, ok := s.Sizes[sz.Code]; ok {
		return nil, domain.ErrMasterExists
	}
	s.Sizes[sz.Code] = sz
	return &sz, nil
}

func (s *CatalogStore) UpdateSizeName(ctx context.Context, code, name string) (*domain.Size, error) {
	_ = ctx
	code = domain.NormalizeCode(code)
	sz, ok := s.Sizes[code]
	if !ok {
		return nil, domain.ErrMasterNotFound
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ports.Invalid("name is required")
	}
	sz.Name = name
	s.Sizes[code] = sz
	return &sz, nil
}

func (s *CatalogStore) DeleteSize(ctx context.Context, code string) error {
	_ = ctx
	code = domain.NormalizeCode(code)
	if _, ok := s.Sizes[code]; !ok {
		return domain.ErrMasterNotFound
	}
	for _, c := range s.VariantSizeCodes {
		if c == code {
			return domain.ErrMasterInUse
		}
	}
	delete(s.Sizes, code)
	return nil
}

func (s *CatalogStore) ListEditions(ctx context.Context) ([]domain.Edition, error) {
	_ = ctx
	out := make([]domain.Edition, 0, len(s.Editions))
	for _, e := range s.Editions {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func (s *CatalogStore) GetEdition(ctx context.Context, code string) (*domain.Edition, error) {
	_ = ctx
	e, ok := s.Editions[domain.NormalizeCode(code)]
	if !ok {
		return nil, domain.ErrMasterNotFound
	}
	return &e, nil
}

func (s *CatalogStore) CreateEdition(ctx context.Context, e domain.Edition) (*domain.Edition, error) {
	_ = ctx
	e.Code = domain.NormalizeCode(e.Code)
	e.Name = strings.TrimSpace(e.Name)
	if !domain.ValidSegmentCode(e.Code) {
		return nil, ports.Invalid(fmt.Sprintf("invalid edition code %q", e.Code))
	}
	if e.Name == "" {
		return nil, ports.Invalid("name is required")
	}
	if _, ok := s.Editions[e.Code]; ok {
		return nil, domain.ErrMasterExists
	}
	s.Editions[e.Code] = e
	return &e, nil
}

func (s *CatalogStore) UpdateEditionName(ctx context.Context, code, name string) (*domain.Edition, error) {
	_ = ctx
	code = domain.NormalizeCode(code)
	e, ok := s.Editions[code]
	if !ok {
		return nil, domain.ErrMasterNotFound
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ports.Invalid("name is required")
	}
	e.Name = name
	s.Editions[code] = e
	return &e, nil
}

func (s *CatalogStore) DeleteEdition(ctx context.Context, code string) error {
	_ = ctx
	code = domain.NormalizeCode(code)
	if _, ok := s.Editions[code]; !ok {
		return domain.ErrMasterNotFound
	}
	for _, c := range s.VariantEditionCodes {
		if c == code {
			return domain.ErrMasterInUse
		}
	}
	delete(s.Editions, code)
	return nil
}

func listTerms(m map[string]domain.CatalogTerm, seedOrder []domain.CatalogTerm) []domain.CatalogTerm {
	out := make([]domain.CatalogTerm, 0, len(m))
	seen := make(map[string]struct{}, len(m))
	for _, t := range seedOrder {
		if cur, ok := m[t.Code]; ok {
			out = append(out, cur)
			seen[t.Code] = struct{}{}
		}
	}
	rest := make([]domain.CatalogTerm, 0)
	for code, t := range m {
		if _, ok := seen[code]; !ok {
			rest = append(rest, t)
		}
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i].Code < rest[j].Code })
	return append(out, rest...)
}

func (s *CatalogStore) ListSubCategories(ctx context.Context) ([]domain.CatalogTerm, error) {
	_ = ctx
	return listTerms(s.SubCategories, domain.SeedSubCategories), nil
}

func (s *CatalogStore) ListBagStyles(ctx context.Context) ([]domain.CatalogTerm, error) {
	_ = ctx
	return listTerms(s.BagStyles, domain.SeedBagStyles), nil
}

func (s *CatalogStore) ListTargets(ctx context.Context) ([]domain.CatalogTerm, error) {
	_ = ctx
	return listTerms(s.Targets, domain.SeedTargets), nil
}
