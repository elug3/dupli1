package domain

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// SKU segment separator for the luxury-fashion naming format.
const SKUSeparator = "_"

var (
	brandCodeRe = regexp.MustCompile(`^[A-Z]{2,3}$`)
	// Style, color, edition, and size: uppercase alphanumeric, 1–12 chars.
	segmentCodeRe = regexp.MustCompile(`^[A-Z0-9]{1,12}$`)
)

// SKUParts holds the normalized segments of a human SKU.
// Edition (VariantCode) is optional; empty means omitted from the composed string.
type SKUParts struct {
	BrandCode   string
	StyleCode   string
	ColorCode   string
	EditionCode string // VariantCode segment; construction/edition only
	SizeCode    string
}

// NormalizeCode uppercases and strips non-alphanumeric characters.
func NormalizeCode(value string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ValidBrandCode reports whether code is a 2–3 letter brand code.
func ValidBrandCode(code string) bool {
	return brandCodeRe.MatchString(code)
}

// ValidSegmentCode reports whether code is a valid style/color/edition/size code.
func ValidSegmentCode(code string) bool {
	return segmentCodeRe.MatchString(code)
}

// ValidateSKUParts checks every required segment and optional edition.
func ValidateSKUParts(p SKUParts) error {
	if !ValidBrandCode(p.BrandCode) {
		return fmt.Errorf("invalid brand code %q: must be 2–3 uppercase letters", p.BrandCode)
	}
	if !ValidSegmentCode(p.StyleCode) {
		return fmt.Errorf("invalid style code %q", p.StyleCode)
	}
	if !ValidSegmentCode(p.ColorCode) {
		return fmt.Errorf("invalid color code %q", p.ColorCode)
	}
	if p.EditionCode != "" && !ValidSegmentCode(p.EditionCode) {
		return fmt.Errorf("invalid edition (variant) code %q", p.EditionCode)
	}
	if !ValidSegmentCode(p.SizeCode) {
		return fmt.Errorf("invalid size code %q", p.SizeCode)
	}
	return nil
}

// BuildSKU composes a deterministic human SKU from validated parts.
// Format: Brand_Style_Color[_Edition]_Size
func BuildSKU(p SKUParts) (string, error) {
	p.BrandCode = NormalizeCode(p.BrandCode)
	p.StyleCode = NormalizeCode(p.StyleCode)
	p.ColorCode = NormalizeCode(p.ColorCode)
	p.EditionCode = NormalizeCode(p.EditionCode)
	p.SizeCode = NormalizeCode(p.SizeCode)
	if err := ValidateSKUParts(p); err != nil {
		return "", err
	}
	parts := []string{p.BrandCode, p.StyleCode, p.ColorCode}
	if p.EditionCode != "" {
		parts = append(parts, p.EditionCode)
	}
	parts = append(parts, p.SizeCode)
	return strings.Join(parts, SKUSeparator), nil
}

// ParseSKU splits a human SKU into segments.
// Accepts 4 segments (no edition) or 5 segments (with edition).
func ParseSKU(sku string) (SKUParts, error) {
	sku = strings.TrimSpace(sku)
	if sku == "" {
		return SKUParts{}, fmt.Errorf("empty sku")
	}
	segs := strings.Split(sku, SKUSeparator)
	var p SKUParts
	switch len(segs) {
	case 4:
		p = SKUParts{
			BrandCode: segs[0],
			StyleCode: segs[1],
			ColorCode: segs[2],
			SizeCode:  segs[3],
		}
	case 5:
		p = SKUParts{
			BrandCode:   segs[0],
			StyleCode:   segs[1],
			ColorCode:   segs[2],
			EditionCode: segs[3],
			SizeCode:    segs[4],
		}
	default:
		return SKUParts{}, fmt.Errorf("sku %q: want 4 or 5 underscore-separated segments", sku)
	}
	if err := ValidateSKUParts(p); err != nil {
		return SKUParts{}, err
	}
	return p, nil
}

// OptionCode is the shared 3-char color/size code used by the legacy SKU helper.
// Prefer NormalizeCode + master-table lookup for the luxury SKU format.
func OptionCode(value string) string {
	code := NormalizeCode(value)
	if len(code) > 3 {
		code = code[:3]
	}
	for len(code) < 3 && code != "" {
		code += "X"
	}
	return code
}

// BuildVariantSKUBase builds the legacy candidate human SKU ({productId}-{color}-{size})
// used when the parent product has no brand_code/style_code yet.
// Prefer BuildSKU when normalized codes are available.
func BuildVariantSKUBase(productID, color, size string) string {
	parts := []string{productID}
	if c := OptionCode(color); c != "" {
		parts = append(parts, c)
	}
	if sz := OptionCode(size); sz != "" {
		parts = append(parts, sz)
	}
	base := strings.Join(parts, "-")
	if base == productID {
		base = productID + "-VAR"
	}
	return base
}

// StyleCodeFromProductID derives a default style code from a legacy parent id
// such as BOT-001 → S001. Returns empty when the id does not match.
func StyleCodeFromProductID(productID string) string {
	productID = strings.TrimSpace(productID)
	i := strings.LastIndex(productID, "-")
	if i < 0 || i+1 >= len(productID) {
		return ""
	}
	suffix := NormalizeCode(productID[i+1:])
	if suffix == "" {
		return ""
	}
	// Pad numeric suffixes to at least 3 digits: 1 → S001, 12 → S012.
	if isAllDigits(suffix) {
		for len(suffix) < 3 {
			suffix = "0" + suffix
		}
		return "S" + suffix
	}
	return suffix
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
