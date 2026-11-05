package domain

import (
	"errors"
	"strings"
)

// BaseColor is a plain color a shopper recognizes (Black, Brown), shared by
// every brand. A color master row (Color) is a brand's colorway: its code is
// what the sku carries (1139, F0002) and stays distinct even
// when two colorways are both "black". Each colorway points at one base
// color, or two for a two-tone colorway (black body, brown trim), and the
// storefront filters and draws swatches by base color.
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

// ErrBase2WithoutBase is returned when a colorway has a second base color but no first.
var ErrBase2WithoutBase = errors.New("base2Code needs baseCode: a two-tone colorway needs its main base color first")

// ErrBase2RepeatsBase is returned when both base colors of a colorway are the same.
var ErrBase2RepeatsBase = errors.New("base2Code must differ from baseCode")

// ErrHex2Retired is returned when a client sets hex2 on a colorway. A
// two-tone colorway names its second color with base2Code instead.
var ErrHex2Retired = errors.New("hex2 is retired: give a two-tone colorway a base2Code instead")

// SeedBaseColors is the plain-color palette, the fills the storefront used
// before colors carried one. Each SeedColors row is its own base color
// (BLK → BLK), so a seed colorway's swatch does not change.
func SeedBaseColors() []BaseColor {
	out := make([]BaseColor, 0, len(SeedColors))
	for _, c := range SeedColors {
		out = append(out, BaseColor{Code: c.Code, Name: c.Name, Hex: c.Hex})
	}
	return out
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

// NormalizeBases normalizes a colorway's base color codes and checks that a
// second one only comes with a first and differs from it. Whether the codes
// exist is the store's check.
func (c *Color) NormalizeBases() error {
	c.BaseCode = NormalizeCode(c.BaseCode)
	c.Base2Code = NormalizeCode(c.Base2Code)
	if c.Base2Code != "" && c.BaseCode == "" {
		return ErrBase2WithoutBase
	}
	if c.Base2Code != "" && c.Base2Code == c.BaseCode {
		return ErrBase2RepeatsBase
	}
	return nil
}
