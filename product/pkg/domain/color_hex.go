package domain

import (
	"errors"
	"strings"
)

// ErrInvalidHex is returned for a swatch color that is not "#rrggbb".
var ErrInvalidHex = errors.New(`hex must be a 6-digit color such as "#1f2a44"`)

// ErrHex2Removed is returned when a client sets hex2 on a color. A color
// master row is one color; there is no second fill.
var ErrHex2Removed = errors.New("hex2 is not supported: a color master row is one color")

// ErrBase2Removed is returned when a client sets base2Code on a color. A
// color master row is one color, so it has one base color.
var ErrBase2Removed = errors.New("base2Code is not supported: a color master row is one color")

// RejectSecondColor refuses a non-empty hex2 or base2Code in a color write.
// An empty or absent one is accepted and ignored: it asks for no second
// color, which is the only kind there is. Older clients still send hex2 "".
func RejectSecondColor(hex2, base2Code *string) error {
	if hex2 != nil && strings.TrimSpace(*hex2) != "" {
		return ErrHex2Removed
	}
	if base2Code != nil && strings.TrimSpace(*base2Code) != "" {
		return ErrBase2Removed
	}
	return nil
}

// NormalizeHex returns s as lowercase "#rrggbb". The leading "#" is optional
// on input; "" stays "" (no swatch color).
func NormalizeHex(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", nil
	}
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return "", ErrInvalidHex
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return "", ErrInvalidHex
		}
	}
	return "#" + s, nil
}

// NormalizeSwatch normalizes Hex in place.
func (c *Color) NormalizeSwatch() error {
	hex, err := NormalizeHex(c.Hex)
	if err != nil {
		return err
	}
	c.Hex = hex
	return nil
}

// Apply returns c with the patch's set fields applied.
func (p ColorPatch) Apply(c Color) Color {
	if p.Name != nil {
		c.Name = *p.Name
	}
	if p.Hex != nil {
		c.Hex = *p.Hex
	}
	if p.BaseCode != nil {
		c.BaseCode = *p.BaseCode
	}
	return c
}
