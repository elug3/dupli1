package domain

import "testing"

func TestNormalizeColor2(t *testing.T) {
	for name, tc := range map[string]struct {
		color, color2, want string
		err                 bool
	}{
		"none":         {"1081", "", "", false},
		"normalized":   {"1081", " brn ", "BRN", false},
		"clear marker": {"1081", Color2Clear, Color2Clear, false},
		"same":         {"BLK", "blk", "", true},
		"no code left": {"BLK", "!!!", "", true},
		"too long":     {"BLK", "ABCDEFGHIJKLM", "", true},
	} {
		v := Variant{ColorCode: tc.color, Color2Code: tc.color2}
		err := NormalizeColor2(&v)
		if (err != nil) != tc.err || (!tc.err && v.Color2Code != tc.want) {
			t.Errorf("%s: got %q, %v", name, v.Color2Code, err)
		}
	}
}

func TestMergeUpdateColor2(t *testing.T) {
	existing := Variant{SKU: "BOT_CAS001_1081_OS", ColorCode: "1081", Color2Code: "BRN"}
	for name, tc := range map[string]struct{ in, want string }{
		"omitted keeps": {"", "BRN"},
		"replaces":      {"TAN", "TAN"},
		"clears":        {Color2Clear, ""},
	} {
		if got := existing.MergeUpdate(Variant{Color2Code: tc.in}).Color2Code; got != tc.want {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
}
