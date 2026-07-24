package memory

import (
	"fmt"
	"sort"
	"strings"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

// CatalogStore is an in-memory ports.CatalogStore for tests.
type CatalogStore struct {
	Brands   map[string]domain.Brand
	Styles   map[string]domain.Style // key: brandCode+"|"+code
	Colors   map[string]domain.Color
	Sizes    map[string]domain.Size
	Editions map[string]domain.Edition

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
		Sizes:               map[string]domain.Size{},
		Editions:            map[string]domain.Edition{},
		ProductBrandStyles:  map[string]string{},
		VariantColorCodes:   map[string]string{},
		VariantSizeCodes:    map[string]string{},
		VariantEditionCodes: map[string]string{},
	}
	for _, b := range domain.SeedBrands {
		s.Brands[b.Code] = b
	}
	for _, c := range domain.SeedColors {
		s.Colors[c.Code] = c
	}
	for _, sz := range domain.SeedSizes {
		s.Sizes[sz.Code] = sz
	}
	for _, e := range domain.SeedEditions {
		s.Editions[e.Code] = e
	}
	return s
}

func styleKey(brand, code string) string {
	return brand + "|" + code
}

func (s *CatalogStore) ListBrands() ([]domain.Brand, error) {
	out := make([]domain.Brand, 0, len(s.Brands))
	for _, b := range s.Brands {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func (s *CatalogStore) GetBrand(code string) (*domain.Brand, error) {
	b, ok := s.Brands[domain.NormalizeCode(code)]
	if !ok {
		return nil, domain.ErrMasterNotFound
	}
	return &b, nil
}

func (s *CatalogStore) CreateBrand(b domain.Brand) (*domain.Brand, error) {
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

func (s *CatalogStore) UpdateBrandName(code, name string) (*domain.Brand, error) {
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

func (s *CatalogStore) DeleteBrand(code string) error {
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

func (s *CatalogStore) ListStyles(brandCode string) ([]domain.Style, error) {
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

func (s *CatalogStore) GetStyle(brandCode, code string) (*domain.Style, error) {
	st, ok := s.Styles[styleKey(domain.NormalizeCode(brandCode), domain.NormalizeCode(code))]
	if !ok {
		return nil, domain.ErrMasterNotFound
	}
	return &st, nil
}

func (s *CatalogStore) CreateStyle(st domain.Style) (*domain.Style, error) {
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

func (s *CatalogStore) UpdateStyleName(brandCode, code, name string) (*domain.Style, error) {
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

func (s *CatalogStore) DeleteStyle(brandCode, code string) error {
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

func (s *CatalogStore) ListColors() ([]domain.Color, error) {
	out := make([]domain.Color, 0, len(s.Colors))
	for _, c := range s.Colors {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func (s *CatalogStore) GetColor(code string) (*domain.Color, error) {
	c, ok := s.Colors[domain.NormalizeCode(code)]
	if !ok {
		return nil, domain.ErrMasterNotFound
	}
	return &c, nil
}

func (s *CatalogStore) CreateColor(c domain.Color) (*domain.Color, error) {
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
	s.Colors[c.Code] = c
	return &c, nil
}

func (s *CatalogStore) UpdateColorName(code, name string) (*domain.Color, error) {
	code = domain.NormalizeCode(code)
	c, ok := s.Colors[code]
	if !ok {
		return nil, domain.ErrMasterNotFound
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ports.Invalid("name is required")
	}
	c.Name = name
	s.Colors[code] = c
	return &c, nil
}

func (s *CatalogStore) DeleteColor(code string) error {
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

func (s *CatalogStore) ListSizes() ([]domain.Size, error) {
	out := make([]domain.Size, 0, len(s.Sizes))
	for _, sz := range s.Sizes {
		out = append(out, sz)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func (s *CatalogStore) GetSize(code string) (*domain.Size, error) {
	sz, ok := s.Sizes[domain.NormalizeCode(code)]
	if !ok {
		return nil, domain.ErrMasterNotFound
	}
	return &sz, nil
}

func (s *CatalogStore) CreateSize(sz domain.Size) (*domain.Size, error) {
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

func (s *CatalogStore) UpdateSizeName(code, name string) (*domain.Size, error) {
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

func (s *CatalogStore) DeleteSize(code string) error {
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

func (s *CatalogStore) ListEditions() ([]domain.Edition, error) {
	out := make([]domain.Edition, 0, len(s.Editions))
	for _, e := range s.Editions {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func (s *CatalogStore) GetEdition(code string) (*domain.Edition, error) {
	e, ok := s.Editions[domain.NormalizeCode(code)]
	if !ok {
		return nil, domain.ErrMasterNotFound
	}
	return &e, nil
}

func (s *CatalogStore) CreateEdition(e domain.Edition) (*domain.Edition, error) {
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

func (s *CatalogStore) UpdateEditionName(code, name string) (*domain.Edition, error) {
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

func (s *CatalogStore) DeleteEdition(code string) error {
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
