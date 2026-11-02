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
	c := Color{Hex: "0A0A0A", Hex2: "#6B4226"}
	if err := c.NormalizeSwatch(); err != nil || c.Hex != "#0a0a0a" || c.Hex2 != "#6b4226" {
		t.Fatalf("two-tone: %+v %v", c, err)
	}
	if err := (&Color{Hex2: "#ffffff"}).NormalizeSwatch(); err != ErrHex2WithoutHex {
		t.Fatalf("hex2 without hex: got %v", err)
	}
	if err := (&Color{}).NormalizeSwatch(); err != nil {
		t.Fatalf("no swatch: %v", err)
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
