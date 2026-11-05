package domain

import (
	"errors"
	"fmt"
	"strings"
)

// A sku in two colors (black body, brown trim) names its second color with
// Variant.Color2Code: another color master code, each master row being one
// color. The second color is not part of the sku string and can be set,
// changed or cleared on an existing variant. See
// docs/product-color-swatches.md.

// Color2Clear is the in-process marker for "clear the second color" on a
// partial variant update, where an empty field means "keep". The handler
// turns an explicit "color2Code": "" into it.
const Color2Clear = "-"

// ErrInvalidColor2 is the class of a bad second color (a 400).
var ErrInvalidColor2 = errors.New("invalid color2Code")

// ErrColor2RepeatsColor is returned when a variant's second color is its
// first color.
var ErrColor2RepeatsColor = fmt.Errorf("%w: it must differ from colorCode", ErrInvalidColor2)

// NormalizeColor2 normalizes v.Color2Code and checks it is a valid code that
// differs from the first color. Color2Clear and "" are left as they are.
// Whether the code exists is the store's check.
func NormalizeColor2(v *Variant) error {
	if v == nil || v.Color2Code == Color2Clear {
		return nil
	}
	raw := v.Color2Code
	v.Color2Code = NormalizeCode(raw)
	if v.Color2Code == "" {
		if strings.TrimSpace(raw) != "" {
			return fmt.Errorf("%w %q", ErrInvalidColor2, raw)
		}
		return nil
	}
	if !ValidSegmentCode(v.Color2Code) {
		return fmt.Errorf("%w %q", ErrInvalidColor2, v.Color2Code)
	}
	if v.Color2Code == NormalizeCode(v.ColorCode) {
		return ErrColor2RepeatsColor
	}
	return nil
}

// ExplicitColor2Clear reports whether a raw "color2Code" JSON value asks to
// clear the second color: present and blank.
func ExplicitColor2Clear(raw *string) bool {
	return raw != nil && strings.TrimSpace(*raw) == ""
}
