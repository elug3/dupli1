package domain

import (
	"fmt"
	"strings"
)

// CatalogTerm is a merchandising master-catalog entry (code → display name).
// Distinct from SKU segment masters (Brand / Style / Color): these classify
// bag products for storefront filters, not human SKU composition.
type CatalogTerm struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// MasterCatalog is the bag merchandising taxonomy returned by
// GET /api/v1/products/catalog/master.
type MasterCatalog struct {
	SubCategories []CatalogTerm `json:"subCategories"`
	Styles        []CatalogTerm `json:"styles"`
	Targets       []CatalogTerm `json:"targets"`
}

// Bag subcategory seeds (under category=bags).
var SeedSubCategories = []CatalogTerm{
	{Code: "handbags", Name: "Handbags"},
	{Code: "tote", Name: "Tote"},
	{Code: "shoulder", Name: "Shoulder"},
	{Code: "cross", Name: "Crossbody"},
	{Code: "mini", Name: "Mini"},
}

// Bag occasion / look style seeds (not SKU design-family styles).
var SeedBagStyles = []CatalogTerm{
	{Code: "casual", Name: "Casual"},
	{Code: "evening", Name: "Evening"},
	{Code: "business", Name: "Business"},
	{Code: "weekend", Name: "Weekend"},
	{Code: "statement", Name: "Statement"},
}

// Audience / target seeds.
var SeedTargets = []CatalogTerm{
	{Code: "men", Name: "Men"},
	{Code: "women", Name: "Women"},
	{Code: "kids", Name: "Kids"},
}

// DefaultMasterCatalog returns the seeded bag taxonomy.
func DefaultMasterCatalog() MasterCatalog {
	return MasterCatalog{
		SubCategories: append([]CatalogTerm(nil), SeedSubCategories...),
		Styles:        append([]CatalogTerm(nil), SeedBagStyles...),
		Targets:       append([]CatalogTerm(nil), SeedTargets...),
	}
}

// NormalizeTaxonomyCode lowercases and trims a merchandising code.
func NormalizeTaxonomyCode(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

func lookupTerm(seeds []CatalogTerm, code string) (CatalogTerm, bool) {
	n := NormalizeTaxonomyCode(code)
	if n == "" {
		return CatalogTerm{}, false
	}
	for _, t := range seeds {
		if t.Code == n || strings.EqualFold(t.Name, strings.TrimSpace(code)) {
			return t, true
		}
	}
	return CatalogTerm{}, false
}

// NormalizeSubCategory returns the canonical subcategory code, or "" if blank.
// Unknown values return ("", false).
func NormalizeSubCategory(code string) (string, bool) {
	if strings.TrimSpace(code) == "" {
		return "", true
	}
	t, ok := lookupTerm(SeedSubCategories, code)
	if !ok {
		return "", false
	}
	return t.Code, true
}

// NormalizeBagStyle returns the canonical bag-style code, or "" if blank.
func NormalizeBagStyle(code string) (string, bool) {
	if strings.TrimSpace(code) == "" {
		return "", true
	}
	t, ok := lookupTerm(SeedBagStyles, code)
	if !ok {
		return "", false
	}
	return t.Code, true
}

// NormalizeTarget returns the canonical target code, or "" if blank.
// Accepts common typo "mem" → "men".
func NormalizeTarget(code string) (string, bool) {
	raw := strings.TrimSpace(code)
	if raw == "" {
		return "", true
	}
	if NormalizeTaxonomyCode(raw) == "mem" {
		raw = "men"
	}
	t, ok := lookupTerm(SeedTargets, raw)
	if !ok {
		return "", false
	}
	return t.Code, true
}

// NormalizeProductTaxonomy validates and normalizes SubCategory / Style / Target
// on a product. Empty values are allowed; unknown codes return an error.
func NormalizeProductTaxonomy(p *Product) error {
	if p == nil {
		return nil
	}
	sc, ok := NormalizeSubCategory(p.SubCategory)
	if !ok {
		return fmt.Errorf("invalid subcategory %q", p.SubCategory)
	}
	p.SubCategory = sc

	st, ok := NormalizeBagStyle(p.Style)
	if !ok {
		return fmt.Errorf("invalid style %q", p.Style)
	}
	p.Style = st

	tg, ok := NormalizeTarget(p.Target)
	if !ok {
		return fmt.Errorf("invalid target %q", p.Target)
	}
	p.Target = tg
	return nil
}
