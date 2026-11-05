package domain

import "testing"

func TestNormalizeHex(t *testing.T) {
	cases := map[string]struct {
		want string
		ok   bool
	}{
		"":          {"", true},
		"#1F2A44":   {"#1f2a44", true},
		"1f2a44":    {"#1f2a44", true},
		" #abcdef ": {"#abcdef", true},
		"#abc":      {"", false},
		"#12345g":   {"", false},
		"navy":      {"", false},
	}
	for in, tc := range cases {
		got, err := NormalizeHex(in)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("NormalizeHex(%q) = %q, %v; want %q ok=%v", in, got, err, tc.want, tc.ok)
		}
	}
}

func TestColorNormalizeSwatch(t *testing.T) {
	c := Color{Hex: "0A0A0A"}
	if err := c.NormalizeSwatch(); err != nil || c.Hex != "#0a0a0a" {
		t.Fatalf("solid: %+v %v", c, err)
	}
	if err := (&Color{Hex: "ivory"}).NormalizeSwatch(); err != ErrInvalidHex {
		t.Fatalf("bad hex: got %v", err)
	}
	if err := (&Color{}).NormalizeSwatch(); err != nil {
		t.Fatalf("no swatch: %v", err)
	}
}

// A color master row is one color: a second fill or base color is refused,
// an empty or absent one (what older clients send) is not.
func TestRejectSecondColor(t *testing.T) {
	s := func(v string) *string { return &v }
	for name, tc := range map[string]struct {
		hex2, base2 *string
		want        error
	}{
		"absent":     {nil, nil, nil},
		"empty":      {s(""), s(" "), nil},
		"hex2":       {s("#111111"), nil, ErrHex2Removed},
		"base2Code":  {nil, s("BRN"), ErrBase2Removed},
		"hex2 first": {s("#111111"), s("BRN"), ErrHex2Removed},
	} {
		if got := RejectSecondColor(tc.hex2, tc.base2); got != tc.want {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
}

func TestSeedColorsHaveValidHex(t *testing.T) {
	for _, c := range SeedColors {
		got, err := NormalizeHex(c.Hex)
		if err != nil || got != c.Hex || c.Hex == "" {
			t.Errorf("seed %s hex %q: %v", c.Code, c.Hex, err)
		}
	}
}
