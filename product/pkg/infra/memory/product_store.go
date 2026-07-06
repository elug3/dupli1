package memory

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/elug3/dupli1/product/pkg/domain"
)

// ProductStore is an in-memory implementation of ports.ProductStore, for use in tests.
type ProductStore struct {
	Products []domain.Product
	Variants []domain.Variant
}

func NewProductStore() *ProductStore {
	return &ProductStore{}
}

func (s *ProductStore) variantsFor(productID string) []domain.Variant {
	var out []domain.Variant
	for _, v := range s.Variants {
		if v.ProductID == productID {
			out = append(out, v)
		}
	}
	return out
}

func (s *ProductStore) enrich(products []domain.Product, includeVariants bool) {
	for i := range products {
		products[i].EnrichFromVariants(s.variantsFor(products[i].ID), includeVariants)
	}
}

func (s *ProductStore) SearchProducts(filter map[string]string) ([]domain.Product, error) {
	var results []domain.Product
	for _, p := range s.Products {
		if category := filter["category"]; category != "" && p.Category != category {
			continue
		}
		if status := filter["status"]; status != "" && p.Status != status {
			continue
		}
		if brand := filter["brand"]; brand != "" && !strings.Contains(strings.ToLower(p.Brand), strings.ToLower(brand)) {
			continue
		}
		if material := filter["material"]; material != "" && p.Material != material {
			continue
		}
		if tags := filter["tags"]; tags != "" && !hasAllTags(p.Tags, tags) {
			continue
		}
		variants := s.variantsFor(p.ID)
		if color := filter["color"]; color != "" && !hasActiveOption(variants, "color", color) {
			continue
		}
		if size := filter["size"]; size != "" && !hasActiveOption(variants, "size", size) {
			continue
		}
		results = append(results, p)
	}
	s.enrich(results, false)
	return results, nil
}

func hasActiveOption(variants []domain.Variant, field, want string) bool {
	for _, v := range variants {
		if v.Status != "" && v.Status != "active" {
			continue
		}
		switch field {
		case "color":
			if v.Color == want {
				return true
			}
		case "size":
			if v.Size == want {
				return true
			}
		}
	}
	return false
}

func hasAllTags(have []string, wantCSV string) bool {
	for _, want := range strings.Split(wantCSV, ",") {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}
		found := false
		for _, tag := range have {
			if tag == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (s *ProductStore) ListProducts() ([]domain.Product, error) {
	return s.SearchProducts(nil)
}

func (s *ProductStore) GetProduct(id string) (*domain.Product, error) {
	for _, p := range s.Products {
		if p.ID == id {
			out := p
			out.EnrichFromVariants(s.variantsFor(id), true)
			return &out, nil
		}
	}
	return nil, fmt.Errorf("product not found: %s", id)
}

func (s *ProductStore) GetActiveProduct(id string) (*domain.Product, error) {
	p, err := s.GetProduct(id)
	if err != nil {
		return nil, err
	}
	if p.Status != "active" {
		return nil, fmt.Errorf("product not found: %s", id)
	}
	active := make([]domain.Variant, 0, len(p.Variants))
	for _, v := range p.Variants {
		if v.Status == "active" {
			active = append(active, v)
		}
	}
	p.EnrichFromVariants(active, true)
	return p, nil
}

func brandPrefix(brand string) string {
	fields := strings.Fields(strings.TrimSpace(brand))
	word := "PRD"
	if len(fields) > 0 {
		word = fields[0]
	}
	runes := []rune(strings.ToUpper(word))
	if len(runes) > 3 {
		runes = runes[:3]
	}
	for len(runes) < 3 {
		runes = append(runes, 'X')
	}
	return string(runes)
}

func (s *ProductStore) nextProductID(brand string) string {
	prefix := brandPrefix(brand)
	max := 0
	for _, p := range s.Products {
		if strings.HasPrefix(p.ID, prefix+"-") {
			if n, err := strconv.Atoi(p.ID[len(prefix)+1:]); err == nil && n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("%s-%03d", prefix, max+1)
}

func (s *ProductStore) CreateProduct(p domain.Product) (*domain.Product, error) {
	if p.ID == "" {
		p.ID = s.nextProductID(p.Brand)
	}
	if p.Status == "" {
		p.Status = "active"
	}
	s.Products = append(s.Products, p)

	switch {
	case len(p.Variants) > 0:
		for _, v := range p.Variants {
			v.ProductID = p.ID
			if v.Status == "" {
				v.Status = p.Status
			}
			if _, err := s.CreateVariant(v); err != nil {
				return nil, err
			}
		}
	case p.Color != "" || p.Price > 0 || p.SellingPrice > 0 || len(p.ImageURLs) > 0:
		if _, err := s.CreateVariant(domain.Variant{
			SKU:          p.ID,
			ProductID:    p.ID,
			Color:        p.Color,
			SellingPrice: p.SellingPrice,
			Price:        p.Price,
			Status:       p.Status,
			ImageURLs:    p.ImageURLs,
		}); err != nil {
			return nil, err
		}
	}

	return s.GetProduct(p.ID)
}

func (s *ProductStore) UpdateProduct(p domain.Product) (*domain.Product, error) {
	for i, existing := range s.Products {
		if existing.ID == p.ID {
			p.CreatedAt = existing.CreatedAt
			s.Products[i] = p
			return s.GetProduct(p.ID)
		}
	}
	return nil, fmt.Errorf("product not found: %s", p.ID)
}

func (s *ProductStore) DeleteProduct(id string) error {
	for i, p := range s.Products {
		if p.ID == id {
			s.Products = append(s.Products[:i], s.Products[i+1:]...)
			kept := s.Variants[:0]
			for _, v := range s.Variants {
				if v.ProductID != id {
					kept = append(kept, v)
				}
			}
			s.Variants = kept
			return nil
		}
	}
	return fmt.Errorf("product not found: %s", id)
}

func optionCode(value string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	code := b.String()
	if len(code) > 3 {
		code = code[:3]
	}
	return code
}

func (s *ProductStore) nextVariantSKU(productID, color, size string) string {
	parts := []string{productID}
	if c := optionCode(color); c != "" {
		parts = append(parts, c)
	}
	if sz := optionCode(size); sz != "" {
		parts = append(parts, sz)
	}
	base := strings.Join(parts, "-")
	if base == productID {
		base = productID + "-VAR"
	}
	candidate := base
	for i := 1; ; i++ {
		exists := false
		for _, v := range s.Variants {
			if v.SKU == candidate {
				exists = true
				break
			}
		}
		if !exists {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, i+1)
	}
}

func (s *ProductStore) ListVariants(productID string) ([]domain.Variant, error) {
	return s.variantsFor(productID), nil
}

func (s *ProductStore) GetVariant(sku string) (*domain.Variant, error) {
	for _, v := range s.Variants {
		if v.SKU == sku {
			out := v
			return &out, nil
		}
	}
	return nil, fmt.Errorf("variant not found: %s", sku)
}

func (s *ProductStore) CreateVariant(v domain.Variant) (*domain.Variant, error) {
	if v.ProductID == "" {
		return nil, fmt.Errorf("productId is required")
	}
	found := false
	for _, p := range s.Products {
		if p.ID == v.ProductID {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("product not found: %s", v.ProductID)
	}
	if v.Status == "" {
		v.Status = "active"
	}
	if v.SKU == "" {
		v.SKU = s.nextVariantSKU(v.ProductID, v.Color, v.Size)
	}
	for _, existing := range s.Variants {
		if existing.SKU == v.SKU {
			return nil, fmt.Errorf("variant already exists: %s", v.SKU)
		}
		if existing.ProductID == v.ProductID && existing.Color == v.Color && existing.Size == v.Size {
			return nil, fmt.Errorf("variant option already exists")
		}
	}
	s.Variants = append(s.Variants, v)
	return &v, nil
}

func (s *ProductStore) UpdateVariant(v domain.Variant) (*domain.Variant, error) {
	for i, existing := range s.Variants {
		if existing.SKU == v.SKU {
			v.ProductID = existing.ProductID
			v.CreatedAt = existing.CreatedAt
			s.Variants[i] = v
			return &v, nil
		}
	}
	return nil, fmt.Errorf("variant not found: %s", v.SKU)
}

func (s *ProductStore) DeleteVariant(sku string) error {
	for i, v := range s.Variants {
		if v.SKU == sku {
			s.Variants = append(s.Variants[:i], s.Variants[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("variant not found: %s", sku)
}
