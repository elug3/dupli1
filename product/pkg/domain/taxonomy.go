package domain

import (
	"fmt"
	"strings"
)

// CatalogTerm is a merchandising master-catalog entry (code → display name).
// Distinct from SKU segment masters (Brand / Style / Color): these classify
// products for storefront filters, not human SKU composition.
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

// Category is a top-level kind of product (bags, clothing). It owns its
// subcategories; style and target are shared by every category.
type Category struct {
	Code          string        `json:"code"`
	Name          string        `json:"name"`
	SubCategories []CatalogTerm `json:"subCategories"`
	// Sizes lists the size master codes its variants may use, in display
	// order. Empty means any size (bags use OS and capacity sizes alike).
	Sizes []string `json:"sizes,omitempty"`
}

// AllowsSize reports whether a variant of this category may carry the size
// master code. A category with no size list allows any code.
func (c Category) AllowsSize(code string) bool {
	if len(c.Sizes) == 0 {
		return true
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	for _, s := range c.Sizes {
		if s == code {
			return true
		}
	}
	return false
}

// CheckVariantSize refuses a size the product's category does not sell.
// Unknown (legacy) categories are not checked.
func CheckVariantSize(category, sizeCode string) error {
	c, ok := LookupCategory(category)
	if !ok || len(c.Sizes) == 0 {
		return nil
	}
	if strings.TrimSpace(sizeCode) == "" {
		return fmt.Errorf("sizeCode is required for %s (one of %s)", c.Code, strings.Join(c.Sizes, ", "))
	}
	if !c.AllowsSize(sizeCode) {
		return fmt.Errorf("size %s is not a %s size (one of %s)", strings.ToUpper(strings.TrimSpace(sizeCode)), c.Code, strings.Join(c.Sizes, ", "))
	}
	return nil
}

// DefaultCategory is what a product without a category is validated as. Rows
// written before categories were checked carry "" and are all bags.
const DefaultCategory = "bags"

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
	{Code: "all", Name: "All"},
	{Code: "men", Name: "Men"},
	{Code: "women", Name: "Women"},
	{Code: "kids", Name: "Kids"},
}

// Clothing subcategory seeds (under category=clothing).
var SeedClothingSubCategories = []CatalogTerm{
	{Code: "padded", Name: "Padded Jackets"},
	{Code: "jackets", Name: "Jackets"},
}

// SeedCategories lists every category a product may carry, in storefront
// order. Adding one here is what makes its products creatable.
var SeedCategories = []Category{
	{Code: "bags", Name: "Bags", SubCategories: SeedSubCategories},
	{Code: "clothing", Name: "Clothing", SubCategories: SeedClothingSubCategories, Sizes: ApparelSizes},
}

// ApparelSizes are the sizes clothing is sold in, smallest first: letter
// sizes, then Italian sizes 34–60, each in its short (S), regular, regular
// (R) and long (L) fit, the way Italian houses label tailoring.
var ApparelSizes = append(
	[]string{"XXS", "XS", "S", "M", "L", "XL", "XXL", "XXXL", "4XL"},
	italianSizes(34, 60)...,
)

func italianSizes(from, to int) []string {
	var out []string
	for n := from; n <= to; n += 2 {
		for _, fit := range []string{"S", "", "R", "L"} {
			out = append(out, fmt.Sprintf("%d%s", n, fit))
		}
	}
	return out
}

// Categories returns a copy of SeedCategories.
func Categories() []Category {
	out := make([]Category, len(SeedCategories))
	for i, c := range SeedCategories {
		c.SubCategories = append([]CatalogTerm(nil), c.SubCategories...)
		c.Sizes = append([]string(nil), c.Sizes...)
		out[i] = c
	}
	return out
}

// LookupCategory finds a category by code or display name, case-insensitively.
func LookupCategory(code string) (Category, bool) {
	n := NormalizeTaxonomyCode(code)
	if n == "" {
		return Category{}, false
	}
	for _, c := range SeedCategories {
		if c.Code == n || strings.EqualFold(c.Name, strings.TrimSpace(code)) {
			return c, true
		}
	}
	return Category{}, false
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

// NormalizeSubCategory returns the canonical bag subcategory code, or "" if
// blank. Unknown values return ("", false).
func NormalizeSubCategory(code string) (string, bool) {
	return normalizeSubCategoryIn(SeedSubCategories, code)
}

func normalizeSubCategoryIn(terms []CatalogTerm, code string) (string, bool) {
	if strings.TrimSpace(code) == "" {
		return "", true
	}
	t, ok := lookupTerm(terms, code)
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

// NormalizeProductTaxonomy validates and normalizes Category / SubCategory /
// Style / Target on a product. The subcategory must belong to the product's
// category; a blank category is validated as bags and stays blank. Empty
// values are allowed; unknown codes return an error.
//
// previousCategory is the category the product already carries (""
// on create). An unknown category is accepted only when it is that one
// unchanged, so a row written before categories were checked can still be
// edited; its subcategory is then checked against bags.
func NormalizeProductTaxonomy(p *Product, previousCategory string) error {
	if p == nil {
		return nil
	}
	subTerms := SeedSubCategories
	categoryName := DefaultCategory
	if strings.TrimSpace(p.Category) != "" {
		c, ok := LookupCategory(p.Category)
		switch {
		case ok:
			p.Category = c.Code
			subTerms = c.SubCategories
			categoryName = c.Code
		case strings.TrimSpace(previousCategory) != "" && p.Category == previousCategory:
			// Legacy free-text category: leave it as stored.
		default:
			return fmt.Errorf("invalid category %q", p.Category)
		}
	}

	sc, ok := normalizeSubCategoryIn(subTerms, p.SubCategory)
	if !ok {
		return fmt.Errorf("invalid subcategory %q for category %q", p.SubCategory, categoryName)
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
