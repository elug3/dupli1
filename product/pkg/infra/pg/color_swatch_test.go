package pg

import (
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
)

// Seed colors get their hex, a hex staff changed survives a restart, and
// every variant read carries its color master's swatch hexes.
func TestColorSwatchHex_SeedAndVariantRead(t *testing.T) {
	dsn := requireProductDSN(t)
	pool := freshProductSchema(t, dsn, "color_swatch_test")
	ctx := t.Context()

	store := &ProductSearchStore{pool: pool}
	if err := store.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	stock, err := NewInventoryStore(pool)
	if err != nil {
		t.Fatalf("inventory migrate: %v", err)
	}
	if err := store.SeedMockEcoBag(ctx, stock); err != nil {
		t.Fatalf("seed: %v", err)
	}
	catalog := NewCatalogStore(pool)

	blk, err := catalog.GetColor(ctx, "BLK")
	if err != nil || blk.Hex != "#0a0a0a" || blk.Hex2 != "" {
		t.Fatalf("seeded BLK: %+v %v", blk, err)
	}

	// Staff make BLK two-tone; a restart (migrate again) keeps it.
	blk.Hex, blk.Hex2 = "#111111", "#6b4226"
	if _, err := catalog.UpdateColor(ctx, *blk); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := store.migrate(); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	if got, _ := catalog.GetColor(ctx, "BLK"); got.Hex != "#111111" || got.Hex2 != "#6b4226" {
		t.Fatalf("restart overwrote staff hex: %+v", got)
	}

	v, err := store.GetVariant(ctx, "DUP_ECO01_BLK_OS")
	if err != nil {
		t.Fatalf("get variant: %v", err)
	}
	if v.ColorHex != "#111111" || v.ColorHex2 != "#6b4226" {
		t.Fatalf("variant read: colorHex=%q colorHex2=%q", v.ColorHex, v.ColorHex2)
	}
	p, err := store.GetProduct(ctx, v.ProductID)
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	for _, pv := range p.Variants {
		if pv.ColorCode == "BLK" && pv.ColorHex2 != "#6b4226" {
			t.Fatalf("PDP variant missing two-tone: %+v", pv)
		}
	}

	// A color created without a hex has none, and is listed that way.
	if _, err := catalog.CreateColor(ctx, domain.Color{Code: "XYZ", Name: "Mystery"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	list, err := catalog.ListColors(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, c := range list {
		if c.Code == "XYZ" && (c.Hex != "" || c.Hex2 != "") {
			t.Fatalf("XYZ should have no hex: %+v", c)
		}
	}
}
