package memory

import (
	"fmt"
	"strconv"
	"strings"

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

func (s *ProductStore) SearchProducts(filter map[string]string) ([]domain.Product, int, error) {
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
	total := len(results)
	if limit, ok := atoiFilter(filter, "limit"); ok && limit > 0 {
		offset, _ := atoiFilter(filter, "offset")
		if offset < 0 {
			offset = 0
		}
		if offset >= len(results) {
			results = nil
		} else {
			end := offset + limit
			if end > len(results) {
				end = len(results)
			}
			results = results[offset:end]
		}
	}
	s.enrich(results, false)
	return results, total, nil
}

func atoiFilter(filter map[string]string, key string) (int, bool) {
	raw, ok := filter[key]
	if !ok || raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return n, true
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
	results, _, err := s.SearchProducts(nil)
	return results, err
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
	if code := domain.BrandCodeFromName(brand); code != "" {
		return code
	}
	return "PRD"
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
	domain.AssignProductCodes(&p)
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
			p.CreatedBy = existing.CreatedBy
			// Brand/style codes are immutable after creation.
			p.BrandCode = existing.BrandCode
			p.StyleCode = existing.StyleCode
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

func (s *ProductStore) nextVariantSKU(productID, brandCode, styleCode string, v *domain.Variant) (string, error) {
	base := domain.ComposeVariantSKU(productID, brandCode, styleCode, v)
	candidate := base
	for i := 1; ; i++ {
		exists := false
		for _, existing := range s.Variants {
			if existing.SKU == candidate {
				exists = true
				break
			}
		}
		if !exists {
			return candidate, nil
		}
		if brandCode != "" && styleCode != "" && v != nil && v.ColorCode != "" {
			return "", fmt.Errorf("duplicate sku %s: same brand/style/color/edition/size already exists", base)
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

func (s *ProductStore) GetVariantBySkuID(skuID string) (*domain.Variant, error) {
	for _, v := range s.Variants {
		if v.SkuID == skuID {
			out := v
			return &out, nil
		}
	}
	return nil, fmt.Errorf("variant not found: %s", skuID)
}

func (s *ProductStore) CreateVariant(v domain.Variant) (*domain.Variant, error) {
	if v.ProductID == "" {
		return nil, fmt.Errorf("productId is required")
	}
	var brandCode, styleCode string
	found := false
	for _, p := range s.Products {
		if p.ID == v.ProductID {
			found = true
			brandCode, styleCode = p.BrandCode, p.StyleCode
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("product not found: %s", v.ProductID)
	}
	if v.Status == "" {
		v.Status = "active"
	}
	domain.ResolveVariantCodes(&v)
	if v.SKU == "" {
		sku, err := s.nextVariantSKU(v.ProductID, brandCode, styleCode, &v)
		if err != nil {
			return nil, err
		}
		v.SKU = sku
	}
	if v.SkuID == "" {
		v.SkuID = domain.NewSkuID()
	}
	for _, existing := range s.Variants {
		if existing.SKU == v.SKU {
			return nil, fmt.Errorf("variant already exists: %s", v.SKU)
		}
		if existing.SkuID == v.SkuID {
			return nil, fmt.Errorf("variant already exists: %s", v.SkuID)
		}
		if existing.ProductID == v.ProductID && existing.Color == v.Color && existing.Size == v.Size {
			return nil, fmt.Errorf("variant option already exists")
		}
	}
	s.Variants = append(s.Variants, v)
	return &v, nil
}

// UpdateVariant updates a variant by its (immutable) sku. SkuID is always
// preserved from the existing row regardless of what the caller passed in.
func (s *ProductStore) UpdateVariant(v domain.Variant) (*domain.Variant, error) {
	for i, existing := range s.Variants {
		if existing.SKU == v.SKU {
			v.SkuID = existing.SkuID
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
