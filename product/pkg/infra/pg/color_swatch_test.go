package pg

import (
	"errors"
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
	if err != nil || blk.Hex != "#0a0a0a" {
		t.Fatalf("seeded BLK: %+v %v", blk, err)
	}

	// Staff change BLK's hex; a restart (migrate again) keeps it.
	blk.Hex = "#111111"
	if _, err := catalog.UpdateColor(ctx, *blk); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := store.migrate(); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	if got, _ := catalog.GetColor(ctx, "BLK"); got.Hex != "#111111" {
		t.Fatalf("restart overwrote staff hex: %+v", got)
	}

	v, err := store.GetVariant(ctx, "DUP_ECO01_BLK_OS")
	if err != nil {
		t.Fatalf("get variant: %v", err)
	}
	if v.ColorHex != "#111111" {
		t.Fatalf("variant read: colorHex=%q", v.ColorHex)
	}
	p, err := store.GetProduct(ctx, v.ProductID)
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	for _, pv := range p.Variants {
		if pv.ColorCode == "BLK" && pv.ColorHex != "#111111" {
			t.Fatalf("PDP variant swatch: %+v", pv)
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
		if c.Code == "XYZ" && c.Hex != "" {
			t.Fatalf("XYZ should have no hex: %+v", c)
		}
	}
}

// Colorways point at one base color: seeds are their own base, the swatch
// fill falls back to the base's, a base in use cannot go, and search finds
// a product by base.
func TestBaseColors(t *testing.T) {
	dsn := requireProductDSN(t)
	pool := freshProductSchema(t, dsn, "base_colors_test")
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
	if err := store.migrate(); err != nil { // a restart keeps everything
		t.Fatalf("re-migrate: %v", err)
	}
	catalog := NewCatalogStore(pool)

	if blk, err := catalog.GetColor(ctx, "BLK"); err != nil || blk.BaseCode != "BLK" {
		t.Fatalf("seed colorway base: %+v %v", blk, err)
	}
	if b, err := catalog.GetBaseColor(ctx, "BRN"); err != nil || b.Hex != "#6b4226" {
		t.Fatalf("seed base color: %+v %v", b, err)
	}

	// The seed variant's colorway moves to another base and drops its own
	// hex: the swatch is the base's.
	blk, _ := catalog.GetColor(ctx, "BLK")
	blk.Hex, blk.BaseCode = "", "BRN"
	if _, err := catalog.UpdateColor(ctx, *blk); err != nil {
		t.Fatalf("move base: %v", err)
	}
	v, err := store.GetVariant(ctx, "DUP_ECO01_BLK_OS")
	if err != nil {
		t.Fatalf("get variant: %v", err)
	}
	if v.BaseColorCode != "BRN" || v.ColorHex != "#6b4226" {
		t.Fatalf("variant base/swatch: %+v", v)
	}

	if err := catalog.DeleteBaseColor(ctx, "BRN"); err != domain.ErrMasterInUse {
		t.Fatalf("delete base in use: want ErrMasterInUse, got %v", err)
	}
	if _, err := catalog.CreateColor(ctx, domain.Color{Code: "1139", Name: "블랙", BaseCode: "ZZZ"}); !errors.Is(err, domain.ErrMasterNotFound) {
		t.Fatalf("unknown base: want ErrMasterNotFound, got %v", err)
	}

	for base, want := range map[string]int{"BRN": 1, "GRN": 0} {
		_, total, err := store.SearchProducts(ctx, map[string]string{"base_color": base})
		if err != nil {
			t.Fatalf("search %s: %v", base, err)
		}
		if total != want {
			t.Errorf("search base_color=%s: got %d products, want %d", base, total, want)
		}
	}
}

// A database migrated before colors became one color per row (it has the
// two-tone hex2 / base2_code columns and their check) loses them on the
// next startup; the rest of the colorway is kept.
func TestColorTwoToneColumnsDropped(t *testing.T) {
	dsn := requireProductDSN(t)
	pool := freshProductSchema(t, dsn, "color_two_tone_drop_test")
	ctx := t.Context()

	store := &ProductSearchStore{pool: pool}
	if err := store.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, sql := range []string{
		`ALTER TABLE colors ADD COLUMN hex2 TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE colors ADD COLUMN base2_code TEXT REFERENCES base_colors(code) ON DELETE RESTRICT`,
		`ALTER TABLE colors ADD CONSTRAINT colors_base2_check
			CHECK (base2_code IS NULL OR (base_code IS NOT NULL AND base2_code <> base_code))`,
		`UPDATE colors SET hex2 = '#6b4226', base2_code = 'BRN' WHERE code = 'BLK'`,
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("old schema: %v", err)
		}
	}
	if err := store.migrate(); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'colors'
		  AND column_name IN ('hex2', 'base2_code')`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("two-tone columns left: %d %v", n, err)
	}
	if blk, err := NewCatalogStore(pool).GetColor(ctx, "BLK"); err != nil || blk.BaseCode != "BLK" || blk.Hex != "#0a0a0a" {
		t.Fatalf("BLK after drop: %+v %v", blk, err)
	}
	if err := store.migrate(); err != nil { // idempotent
		t.Fatalf("third migrate: %v", err)
	}
}

// A two-color sku stores its second color in color2_code: reads carry both
// fills, search finds it by either base, the second color can be cleared,
// a color used as a second color cannot be deleted, and the database
// refuses a second color equal to the first.
func TestVariantSecondColor(t *testing.T) {
	dsn := requireProductDSN(t)
	pool := freshProductSchema(t, dsn, "variant_color2_test")
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
	if err := store.migrate(); err != nil { // a restart keeps everything
		t.Fatalf("re-migrate: %v", err)
	}
	catalog := NewCatalogStore(pool)

	v, err := store.GetVariant(ctx, "DUP_ECO01_BLK_OS")
	if err != nil {
		t.Fatalf("get variant: %v", err)
	}
	if v.Color2Code != "" || v.ColorHex2 != "" {
		t.Fatalf("seed variant is one color: %+v", v)
	}

	v.Color2Code = "BRN"
	if _, err := store.UpdateVariant(ctx, *v); err != nil {
		t.Fatalf("set second color: %v", err)
	}
	got, err := store.GetVariant(ctx, v.SKU)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SKU != "DUP_ECO01_BLK_OS" || got.Color2Code != "BRN" || got.BaseColor2Code != "BRN" || got.ColorHex2 != "#6b4226" || got.ColorHex != "#0a0a0a" {
		t.Fatalf("two-color read: %+v", got)
	}
	for base, want := range map[string]int{"BRN": 1, "BLK": 1, "GRN": 0} {
		if _, total, err := store.SearchProducts(ctx, map[string]string{"base_color": base}); err != nil || total != want {
			t.Errorf("search base_color=%s: got %d (%v), want %d", base, total, err, want)
		}
	}
	if err := catalog.DeleteColor(ctx, "BRN"); err != domain.ErrMasterInUse {
		t.Fatalf("delete second color in use: want ErrMasterInUse, got %v", err)
	}
	v.Color2Code = "ZZZ"
	if _, err := store.UpdateVariant(ctx, *v); !errors.Is(err, domain.ErrMasterNotFound) {
		t.Fatalf("unknown second color: want ErrMasterNotFound, got %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE product_variants SET color2_code = color_code WHERE sku = 'DUP_ECO01_BLK_OS'`); err == nil {
		t.Fatal("color2_code = color_code: want check violation")
	}

	v.Color2Code = ""
	if _, err := store.UpdateVariant(ctx, *v); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got, _ := store.GetVariant(ctx, v.SKU); got.Color2Code != "" || got.ColorHex2 != "" {
		t.Fatalf("after clear: %+v", got)
	}
}
