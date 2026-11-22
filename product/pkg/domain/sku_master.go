package domain

import "strings"

// Brand is a master brand entity. Code is 2–3 uppercase letters and never changes.
type Brand struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Style is a master style (design family) scoped to a brand.
type Style struct {
	BrandCode string `json:"brandCode"`
	Code      string `json:"code"`
	Name      string `json:"name"`
}

// Color is a master color entity: a brand's colorway. Code is what the sku
// carries and is reused across all products; two colorways may share a name
// ("블랙") and stay distinct.
//
// A color master row is exactly one color. A combination of two colors is
// not a master row: it never matches another product's variants, so it
// cannot be reused. Combining colors is the sku's job.
//
// BaseCode is the plain color it is (BaseColor). Hex is an optional
// colorway-specific swatch fill ("#rrggbb") that overrides the base color's;
// with neither, the storefront draws a neutral "no color data" swatch.
// See docs/product-color-swatches.md.
type Color struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Hex      string `json:"hex,omitempty"`
	BaseCode string `json:"baseCode,omitempty"`
}

// ColorPatch is a partial color update: a nil field is kept, "" clears a hex
// or the base color.
type ColorPatch struct {
	Name     *string
	Hex      *string
	BaseCode *string
}

// Size is a master size / capacity entity.
type Size struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Edition is a construction / manufacturing variant (SKU VariantCode segment).
// Named Edition in code to avoid confusion with product_variants (sellable rows).
type Edition struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// CategorySizes is every size a category sells, named by its code (48R,
// 4XL). The product service writes these into the size master at startup
// because they follow from the category definitions in code: without them a
// clothing SKU in a size its category allows would pass the category check and
// then fail the master lookup on create. Every other size is reference data,
// loaded by scripts/seed-reference-data.py.
func CategorySizes() []Size {
	var out []Size
	seen := map[string]bool{}
	for _, c := range SeedCategories {
		for _, code := range c.Sizes {
			if !seen[code] {
				seen[code] = true
				out = append(out, Size{Code: code, Name: code})
			}
		}
	}
	return out
}

// DefaultSize is the size a variant without one gets (ResolveVariantCodes):
// a bag sold in one size.
var DefaultSize = Size{Code: "OS", Name: "One Size"}

// CodeSizes is every size the code itself depends on, which the product
// service writes into the size master at startup: DefaultSize, then
// CategorySizes. Without DefaultSize a variant created without a size would
// fail the master lookup on an environment where seed-reference-data.py has
// not run yet.
func CodeSizes() []Size {
	out := []Size{DefaultSize}
	for _, sz := range CategorySizes() {
		if sz.Code != DefaultSize.Code {
			out = append(out, sz)
		}
	}
	return out
}

// BrandCodeFromName derives a brand code from a display name: the first
// word's leading 2–3 letters (same rules as legacy brandPrefix). Stores look
// the name up in the brand master first; this is only the fallback when it is
// not there, and the master check that follows still decides.
func BrandCodeFromName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	return legacyBrandPrefix(trimmed)
}

// ColorCodeFromName derives a color code from a display name or code. Stores
// look the name up in the color master first; this is only the fallback.
func ColorCodeFromName(name string) string {
	code := NormalizeCode(name)
	if code == "" {
		return ""
	}
	// Already looks like a short code (e.g. F0032, BLK).
	if ValidSegmentCode(code) && len(code) <= 6 {
		return code
	}
	return OptionCode(name)
}

// SizeCodeFromName derives a size code from a display name or code. Stores
// look the name up in the size master first; this is only the fallback.
func SizeCodeFromName(name string) string {
	code := NormalizeCode(name)
	if code == "" {
		return ""
	}
	if ValidSegmentCode(code) && len(code) <= 6 {
		return code
	}
	return OptionCode(name)
}

// EditionCodeFromName normalizes an edition code.
func EditionCodeFromName(name string) string {
	code := NormalizeCode(name)
	if code == "" {
		return ""
	}
	if ValidSegmentCode(code) {
		return code
	}
	return ""
}

func legacyBrandPrefix(brand string) string {
	fields := strings.Fields(strings.TrimSpace(brand))
	word := "PRD"
	if len(fields) > 0 {
		word = fields[0]
	}
	runes := []rune(strings.ToUpper(word))
	if len(runes) > 3 {
		runes = runes[:3]
	}
	for len(runes) < 2 {
		runes = append(runes, 'X')
	}
	// Brand codes are 2–3 letters only.
	out := make([]rune, 0, 3)
	for _, r := range runes {
		if r >= 'A' && r <= 'Z' {
			out = append(out, r)
		}
		if len(out) == 3 {
			break
		}
	}
	for len(out) < 2 {
		out = append(out, 'X')
	}
	return string(out)
}
