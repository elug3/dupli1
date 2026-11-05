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
// BaseCode / Base2Code are the plain colors it is (BaseColor): one, or two
// for a two-tone colorway. Hex is an optional colorway-specific swatch fill
// ("#rrggbb") that overrides the base color's; with neither, the storefront
// draws a neutral "no color data" swatch. Hex2 is the retired two-tone fill:
// still read, never set by clients. See docs/product-color-swatches.md.
type Color struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Hex       string `json:"hex,omitempty"`
	Hex2      string `json:"hex2,omitempty"`
	BaseCode  string `json:"baseCode,omitempty"`
	Base2Code string `json:"base2Code,omitempty"`
}

// ColorPatch is a partial color update: a nil field is kept, "" clears a hex
// or a base color. Hex2 may only be cleared (ErrHex2Retired).
type ColorPatch struct {
	Name      *string
	Hex       *string
	Hex2      *string
	BaseCode  *string
	Base2Code *string
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

// Well-known brand seeds (code → display name).
var SeedBrands = []Brand{
	{Code: "DUP", Name: "Dupli1 Studio"},
	{Code: "PR", Name: "Prada"},
	{Code: "BOT", Name: "Bottega Veneta"},
	{Code: "LV", Name: "Louis Vuitton"},
	{Code: "CD", Name: "Dior"},
	{Code: "CH", Name: "Chanel"},
	{Code: "GUC", Name: "Gucci"},
	{Code: "HER", Name: "Hermes"},
	{Code: "CEL", Name: "Celine"},
	{Code: "BAL", Name: "Balenciaga"},
	{Code: "FEN", Name: "Fendi"},
	{Code: "YSL", Name: "Saint Laurent"},
	{Code: "BUR", Name: "Burberry"},
}

// Well-known color seeds.
//
// Hex values match the swatch fills the storefront used before colors carried
// one, so seeding them changes nothing a shopper sees.
var SeedColors = []Color{
	{Code: "BLK", Name: "Black", Hex: "#0a0a0a"},
	{Code: "WHT", Name: "White", Hex: "#ffffff"},
	{Code: "CRM", Name: "Cream", Hex: "#efe6d2"},
	{Code: "BRN", Name: "Brown", Hex: "#6b4226"},
	{Code: "GRN", Name: "Green", Hex: "#3f6b4a"},
	{Code: "RED", Name: "Red", Hex: "#b3261e"},
	{Code: "BLU", Name: "Blue", Hex: "#2f5d9a"},
	{Code: "BGE", Name: "Beige", Hex: "#d8c3a5"},
	{Code: "NVY", Name: "Navy", Hex: "#1f2a44"},
	{Code: "PNK", Name: "Pink", Hex: "#e8b4bc"},
	{Code: "GRY", Name: "Grey", Hex: "#8e8e8e"},
	{Code: "GLD", Name: "Gold", Hex: "#c8a96e"},
	{Code: "SLV", Name: "Silver", Hex: "#c0c3c7"},
	{Code: "ORG", Name: "Orange", Hex: "#d9772b"},
	{Code: "YLW", Name: "Yellow", Hex: "#e6c34a"},
	{Code: "PRP", Name: "Purple", Hex: "#5e3d7a"},
	{Code: "TAN", Name: "Tan", Hex: "#c19a6b"},
	{Code: "OLV", Name: "Olive", Hex: "#6b6b3a"},
}

// Well-known size seeds (apparel + bag capacity shorthand).
var SeedSizes = []Size{
	{Code: "XXS", Name: "XXS"},
	{Code: "XS", Name: "XS"},
	{Code: "S", Name: "S"},
	{Code: "M", Name: "M"},
	{Code: "L", Name: "L"},
	{Code: "XL", Name: "XL"},
	{Code: "XXL", Name: "XXL"},
	{Code: "MIN", Name: "Mini"},
	{Code: "SML", Name: "Small"},
	{Code: "MED", Name: "Medium"},
	{Code: "LRG", Name: "Large"},
	{Code: "OS", Name: "One Size"},
}

// SizeMasterSeeds is every size the size master is seeded with: SeedSizes,
// then each size a category sells that SeedSizes lacks, named by its code
// (48R, 4XL). Without these a clothing SKU in a size its category allows
// still fails the master lookup on create.
func SizeMasterSeeds() []Size {
	out := append([]Size(nil), SeedSizes...)
	seen := make(map[string]bool, len(out))
	for _, s := range out {
		seen[s.Code] = true
	}
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

// Well-known edition (VariantCode) seeds.
var SeedEditions = []Edition{
	{Code: "V", Name: "Standard"},
	{Code: "A", Name: "Alternate construction"},
	{Code: "R", Name: "Limited / special edition"},
}

// BrandCodeFromName resolves a display brand name to a seeded code when possible.
// Falls back to the first-word 2–3 letter prefix (same rules as legacy brandPrefix).
func BrandCodeFromName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	for _, b := range SeedBrands {
		if strings.ToLower(b.Name) == lower {
			return b.Code
		}
		fields := strings.Fields(b.Name)
		if len(fields) > 0 && strings.HasPrefix(lower, strings.ToLower(fields[0])) {
			return b.Code
		}
	}
	return legacyBrandPrefix(trimmed)
}

// ColorCodeFromName resolves a display color name to a seeded code when possible.
func ColorCodeFromName(name string) string {
	code := NormalizeCode(name)
	if code == "" {
		return ""
	}
	for _, c := range SeedColors {
		if c.Code == code {
			return c.Code
		}
		if strings.EqualFold(c.Name, strings.TrimSpace(name)) {
			return c.Code
		}
	}
	// Already looks like a short code (e.g. F0032, BLK).
	if ValidSegmentCode(code) && len(code) <= 6 {
		return code
	}
	return OptionCode(name)
}

// SizeCodeFromName resolves a display size name to a seeded code when possible.
func SizeCodeFromName(name string) string {
	code := NormalizeCode(name)
	if code == "" {
		return ""
	}
	for _, s := range SeedSizes {
		if s.Code == code {
			return s.Code
		}
		if strings.EqualFold(s.Name, strings.TrimSpace(name)) {
			return s.Code
		}
	}
	if ValidSegmentCode(code) && len(code) <= 6 {
		return code
	}
	return OptionCode(name)
}

// EditionCodeFromName resolves a display edition name or code.
func EditionCodeFromName(name string) string {
	code := NormalizeCode(name)
	if code == "" {
		return ""
	}
	for _, e := range SeedEditions {
		if e.Code == code {
			return e.Code
		}
		if strings.EqualFold(e.Name, strings.TrimSpace(name)) {
			return e.Code
		}
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
