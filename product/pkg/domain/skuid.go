package domain

import (
	"strings"
	"unicode"

	"github.com/oklog/ulid/v2"
)

// NewSkuID returns a new canonical, sortable, cross-service sku identifier.
func NewSkuID() string {
	return ulid.Make().String()
}

// OptionCode is the shared 3-char color/size code used to build a human SKU.
// Shared by the pg and memory product stores so they can't diverge again.
func OptionCode(value string) string {
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
	for len(code) < 3 && code != "" {
		code += "X"
	}
	return code
}

// BuildVariantSKUBase builds the candidate human SKU for a variant, before any
// uniqueness suffix is appended by the caller.
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
