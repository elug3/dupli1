package domain

import (
	"errors"
	"strings"
)

// BaseColor is a plain color a shopper recognizes (Black, Brown), shared by
// every brand. A color master row (Color) is a brand's colorway: its code is
// what the sku carries (1139, F0002) and stays distinct even
// when two colorways are both "black". Each colorway points at one base
// color, and the storefront filters and draws swatches by base color.
// See docs/product-color-swatches.md.
type BaseColor struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Hex  string `json:"hex,omitempty"`
}

// BaseColorPatch is a partial base color update: a nil field is kept.
type BaseColorPatch struct {
	Name *string
	Hex  *string
}

// Apply returns b with the patch's set fields applied.
func (p BaseColorPatch) Apply(b BaseColor) BaseColor {
	if p.Name != nil {
		b.Name = *p.Name
	}
	if p.Hex != nil {
		b.Hex = *p.Hex
	}
	return b
}

// Normalize trims and validates a base color: a valid code, a name, and a
// "#rrggbb" hex or none.
func (b *BaseColor) Normalize() error {
	b.Code = NormalizeCode(b.Code)
	b.Name = strings.TrimSpace(b.Name)
	if !ValidSegmentCode(b.Code) {
		return errors.New("invalid base color code " + `"` + b.Code + `"`)
	}
	if b.Name == "" {
		return errors.New("name is required")
	}
	hex, err := NormalizeHex(b.Hex)
	if err != nil {
		return err
	}
	b.Hex = hex
	return nil
}

// NormalizeBases normalizes a colorway's base color code. Whether it exists
// is the store's check.
func (c *Color) NormalizeBases() error {
	c.BaseCode = NormalizeCode(c.BaseCode)
	return nil
}
