package domain

import (
	"fmt"
	"strings"
)

// CatalogTerm is a merchandising master-catalog entry (code → display name).
// Distinct from SKU segment masters (Brand / Style / Color): these classify
// products for storefront filters, not human SKU composition.
//
// Staff edit them (product.master.write); nothing in code lists them. A
// subcategory belongs to one category, and its code is unique across all
// categories. SortOrder orders a list (ascending, then by code).
type CatalogTerm struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Category  string `json:"category,omitempty"`
	SortOrder int    `json:"sortOrder"`
}

// TaxonomyKind names one of the three staff-edited merchandising lists.
type TaxonomyKind string

const (
	TaxonomySubCategory TaxonomyKind = "subcategory"
	TaxonomyBagStyle    TaxonomyKind = "bag_style"
	TaxonomyTarget      TaxonomyKind = "target"
)

// Valid reports whether k is one of the three kinds.
func (k TaxonomyKind) Valid() bool {
	return k == TaxonomySubCategory || k == TaxonomyBagStyle || k == TaxonomyTarget
}

// CatalogTermPatch changes a term's display name and/or sort order. Its code,
// and a subcategory's category, never change: products store the code.
type CatalogTermPatch struct {
	Name      *string
	SortOrder *int
}

// Taxonomy is every merchandising term as the store holds it, which is what
// products are validated against.
type Taxonomy struct {
	SubCategories []CatalogTerm // each carries its Category
	Styles        []CatalogTerm
	Targets       []CatalogTerm
}

// SubCategoriesOf returns one category's subcategories, in list order.
func (t Taxonomy) SubCategoriesOf(category string) []CatalogTerm {
	category = NormalizeTaxonomyCode(category)
	out := []CatalogTerm{}
	for _, sc := range t.SubCategories {
		if sc.Category == category {
			out = append(out, sc)
		}
	}
	return out
}

// ValidTaxonomyCode reports whether code (already normalized) is usable as a
// term code: lowercase letters, digits, '-' or '_', starting with a letter or
// digit, at most 32 characters.
func ValidTaxonomyCode(code string) bool {
	if code == "" || len(code) > 32 {
		return false
	}
	for i, r := range code {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case (r == '-' || r == '_') && i > 0:
		default:
			return false
		}
	}
	return true
}

// MasterCatalog is the bag merchandising taxonomy returned by
// GET /api/v1/products/catalog/master.
type MasterCatalog struct {
	SubCategories []CatalogTerm `json:"subCategories"`
	Styles        []CatalogTerm `json:"styles"`
	Targets       []CatalogTerm `json:"targets"`
}

// Category is a top-level kind of product (bags, clothing), defined in code:
// adding one is what makes its products creatable. It owns its subcategories,
// which staff edit and the catalog service fills in; style and target are
// shared by every category.
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

// SeedCategories lists every category a product may carry, in storefront
// order. Adding one here is what makes its products creatable.
var SeedCategories = []Category{
	{Code: "bags", Name: "Bags"},
	{Code: "clothing", Name: "Clothing", Sizes: ApparelSizes},
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

// Categories returns a copy of SeedCategories, each with the subcategories
// tax holds for it.
func Categories(tax Taxonomy) []Category {
	out := make([]Category, len(SeedCategories))
	for i, c := range SeedCategories {
		c.SubCategories = tax.SubCategoriesOf(c.Code)
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

// NormalizeTaxonomyCode lowercases and trims a merchandising code.
func NormalizeTaxonomyCode(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

func lookupTerm(terms []CatalogTerm, code string) (CatalogTerm, bool) {
	n := NormalizeTaxonomyCode(code)
	if n == "" {
		return CatalogTerm{}, false
	}
	for _, t := range terms {
		if t.Code == n || strings.EqualFold(t.Name, strings.TrimSpace(code)) {
			return t, true
		}
	}
	return CatalogTerm{}, false
}

// NormalizeTerm returns the canonical code for a code or display name in
// terms, or "" if blank. Unknown values return ("", false). For targets the
// common typo "mem" is read as "men".
func NormalizeTerm(kind TaxonomyKind, terms []CatalogTerm, code string) (string, bool) {
	raw := strings.TrimSpace(code)
	if raw == "" {
		return "", true
	}
	if kind == TaxonomyTarget && NormalizeTaxonomyCode(raw) == "mem" {
		raw = "men"
	}
	t, ok := lookupTerm(terms, raw)
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
// The terms come from tax, what staff keep in the store.
//
// previousCategory is the category the product already carries (""
// on create). An unknown category is accepted only when it is that one
// unchanged, so a row written before categories were checked can still be
// edited; its subcategory is then checked against bags.
func NormalizeProductTaxonomy(p *Product, previousCategory string, tax Taxonomy) error {
	if p == nil {
		return nil
	}
	categoryName := DefaultCategory
	if strings.TrimSpace(p.Category) != "" {
		c, ok := LookupCategory(p.Category)
		switch {
		case ok:
			p.Category = c.Code
			categoryName = c.Code
		case strings.TrimSpace(previousCategory) != "" && p.Category == previousCategory:
			// Legacy free-text category: leave it as stored.
		default:
			return fmt.Errorf("invalid category %q", p.Category)
		}
	}

	sc, ok := NormalizeTerm(TaxonomySubCategory, tax.SubCategoriesOf(categoryName), p.SubCategory)
	if !ok {
		return fmt.Errorf("invalid subcategory %q for category %q", p.SubCategory, categoryName)
	}
	p.SubCategory = sc

	st, ok := NormalizeTerm(TaxonomyBagStyle, tax.Styles, p.Style)
	if !ok {
		return fmt.Errorf("invalid style %q", p.Style)
	}
	p.Style = st

	tg, ok := NormalizeTerm(TaxonomyTarget, tax.Targets, p.Target)
	if !ok {
		return fmt.Errorf("invalid target %q", p.Target)
	}
	p.Target = tg
	return nil
}
