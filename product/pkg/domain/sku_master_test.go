package domain

import (
	"strings"
	"testing"
)

// Every size a category sells must be in the size master, or a SKU passes
// the category check and then fails the master lookup on create.
func TestSizeMasterSeedsCoverCategorySizes(t *testing.T) {
	seeds := SizeMasterSeeds()
	codes := make(map[string]bool, len(seeds))
	for _, s := range seeds {
		if codes[s.Code] {
			t.Fatalf("size %s seeded twice", s.Code)
		}
		if !ValidSegmentCode(s.Code) {
			t.Fatalf("size %s is not a valid sku segment", s.Code)
		}
		codes[s.Code] = true
	}
	for _, c := range Categories() {
		for _, code := range c.Sizes {
			if !codes[code] {
				t.Errorf("category %s sells size %s, which the size master is not seeded with", c.Code, code)
			}
		}
	}
	// The original seeds come first, unchanged.
	for i, s := range SeedSizes {
		if seeds[i] != s {
			t.Fatalf("seed %d = %+v, want %+v", i, seeds[i], s)
		}
	}
}

func TestCheckVariantCodesUnchanged(t *testing.T) {
	existing := Variant{SKU: "BOT_CAS001_BLK_V_MED", ColorCode: "BLK", SizeCode: "MED", EditionCode: "V"}
	for _, tc := range []struct {
		name     string
		incoming Variant
		wantErr  string
	}{
		{"omitted", Variant{Status: "draft"}, ""},
		{"same codes", Variant{ColorCode: "BLK", SizeCode: "MED", EditionCode: "V"}, ""},
		{"same code, other case", Variant{ColorCode: "blk"}, ""},
		{"display name only", Variant{Color: "Jet Black"}, ""},
		{"color", Variant{ColorCode: "RED"}, "colorCode"},
		{"size", Variant{SizeCode: "LRG"}, "sizeCode"},
		{"edition", Variant{EditionCode: "R"}, "editionCode"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckVariantCodesUnchanged(existing, tc.incoming)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), existing.SKU) {
				t.Fatalf("want error naming %s and the sku, got %v", tc.wantErr, err)
			}
		})
	}

	// A composed sku without an edition: adding one is a change.
	noEdition := Variant{SKU: "BOT_CAS001_BLK_MED", ColorCode: "BLK", SizeCode: "MED"}
	if err := CheckVariantCodesUnchanged(noEdition, Variant{EditionCode: "R"}); err == nil {
		t.Fatal("adding an edition to a composed sku: want error")
	}

	// A legacy row without codes may be given one.
	if err := CheckVariantCodesUnchanged(Variant{SKU: "BOT-001-GRN"}, Variant{ColorCode: "GRN", SizeCode: "OS"}); err != nil {
		t.Fatalf("legacy backfill: %v", err)
	}
}
