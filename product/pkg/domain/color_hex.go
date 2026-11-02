package domain

import (
	"errors"
	"strings"
)

// ErrInvalidHex is returned for a swatch color that is not "#rrggbb".
var ErrInvalidHex = errors.New(`hex must be a 6-digit color such as "#1f2a44"`)

// ErrHex2WithoutHex is returned when a color has a second hex but no first.
var ErrHex2WithoutHex = errors.New("hex2 needs hex: a two-tone color needs its main color first")

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

// NormalizeSwatch normalizes Hex and Hex2 in place and checks that a second
// color only comes with a first.
func (c *Color) NormalizeSwatch() error {
	hex, err := NormalizeHex(c.Hex)
	if err != nil {
		return err
	}
	hex2, err := NormalizeHex(c.Hex2)
	if err != nil {
		return err
	}
	if hex == "" && hex2 != "" {
		return ErrHex2WithoutHex
	}
	c.Hex, c.Hex2 = hex, hex2
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
	if p.Hex2 != nil {
		c.Hex2 = *p.Hex2
	}
	return c
}
