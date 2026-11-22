package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
)

// The in-memory fixtures are the reference data a real environment loads with
// scripts/seed-reference-data.py, so tests exercise the masters staff start
// from. Either side changing alone fails here.
func TestFixturesMatchReferenceData(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "scripts", "seed", "reference-data.json"))
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Brands     []domain.Brand     `json:"brands"`
		BaseColors []domain.BaseColor `json:"base_colors"`
		Colors     []domain.Color     `json:"colors"`
		Sizes      []domain.Size      `json:"sizes"`
		Editions   []domain.Edition   `json:"editions"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(data.Brands, fixtureBrands) {
		t.Errorf("brands differ:\n json %+v\n go   %+v", data.Brands, fixtureBrands)
	}
	if !reflect.DeepEqual(data.Sizes, fixtureSizes) {
		t.Errorf("sizes differ:\n json %+v\n go   %+v", data.Sizes, fixtureSizes)
	}
	if !reflect.DeepEqual(data.Editions, fixtureEditions) {
		t.Errorf("editions differ:\n json %+v\n go   %+v", data.Editions, fixtureEditions)
	}
	var colors, bases []domain.Color
	for _, c := range fixtureColors {
		colors = append(colors, domain.Color{Code: c.Code, Name: c.Name, Hex: c.Hex, BaseCode: c.Code})
		bases = append(bases, domain.Color{Code: c.Code, Name: c.Name, Hex: c.Hex})
	}
	if !reflect.DeepEqual(data.Colors, colors) {
		t.Errorf("colors differ:\n json %+v\n go   %+v", data.Colors, colors)
	}
	var jsonBases []domain.Color
	for _, b := range data.BaseColors {
		jsonBases = append(jsonBases, domain.Color{Code: b.Code, Name: b.Name, Hex: b.Hex})
	}
	if !reflect.DeepEqual(jsonBases, bases) {
		t.Errorf("base colors differ:\n json %+v\n go   %+v", jsonBases, bases)
	}
	for _, c := range data.Colors {
		if got, err := domain.NormalizeHex(c.Hex); err != nil || got != c.Hex || c.Hex == "" {
			t.Errorf("color %s hex %q: %v", c.Code, c.Hex, err)
		}
	}
}
