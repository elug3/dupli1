package pg

import (
	"errors"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
)

// Names resolve against the master tables, and every size clothing sells is
// in the size master — with OS, the only sizes the service still seeds at startup.
func TestCreate_RuntimeMasterNamesAndClothingSizes(t *testing.T) {
	dsn := requireProductDSN(t)
	pool := freshProductSchema(t, dsn, "master_names_test")
	ctx := t.Context()

	store := &ProductSearchStore{pool: pool}
	if err := store.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	loadReferenceData(t, pool)
	if _, err := NewInventoryStore(pool); err != nil {
		t.Fatalf("inventory migrate: %v", err)
	}
	catalog := NewCatalogStore(pool)
	if _, err := catalog.CreateBrand(ctx, domain.Brand{Code: "MM", Name: "Maison Margiela"}); err != nil {
		t.Fatal(err)
	}
	for _, st := range []domain.Style{{BrandCode: "MM", Code: "S001", Name: "5AC"}, {BrandCode: "MM", Code: "S002", Name: "Puffer"}} {
		if _, err := catalog.CreateStyle(ctx, st); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := catalog.CreateColor(ctx, domain.Color{Code: "IVR", Name: "Ivory"}); err != nil {
		t.Fatal(err)
	}

	bag, err := store.CreateProduct(ctx, domain.Product{Name: "5AC", Brand: "maison margiela", StyleCode: "S001", Category: "bags", Price: 100000})
	if err != nil {
		t.Fatalf("create by brand name: %v", err)
	}
	if bag.BrandCode != "MM" {
		t.Fatalf("brandCode = %q, want MM", bag.BrandCode)
	}
	v, err := store.CreateVariant(ctx, domain.Variant{ProductID: bag.ID, Color: "Ivory"})
	if err != nil {
		t.Fatalf("create by color name: %v", err)
	}
	if v.ColorCode != "IVR" || v.SKU != "MM_S001_IVR_OS" {
		t.Fatalf("variant = %s / %s, want IVR / MM_S001_IVR_OS", v.ColorCode, v.SKU)
	}
	if _, err := store.CreateVariant(ctx, domain.Variant{ProductID: bag.ID, Color: "Chartreuse"}); !errors.Is(err, domain.ErrMasterNotFound) {
		t.Fatalf("unknown color: want ErrMasterNotFound, got %v", err)
	}

	jacket, err := store.CreateProduct(ctx, domain.Product{Name: "Puffer", BrandCode: "MM", StyleCode: "S002", Category: "clothing", SubCategory: "padded", Price: 900000})
	if err != nil {
		t.Fatal(err)
	}
	clothing, _ := domain.LookupCategory("clothing")
	for _, size := range clothing.Sizes {
		if _, err := store.CreateVariant(ctx, domain.Variant{ProductID: jacket.ID, ColorCode: "BLK", SizeCode: size}); err != nil {
			t.Fatalf("size %s: %v", size, err)
		}
	}
	// Re-seeding the category sizes on restart is a no-op.
	if err := store.migrate(); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
}

// A fresh database holds no brands, colors, base colors or editions, and of
// the sizes only those a category sells: the rest is reference data a script
// loads, so a row staff delete stays deleted across restarts.
func TestMigrate_SeedsOnlyCodeSizes(t *testing.T) {
	dsn := requireProductDSN(t)
	pool := freshProductSchema(t, dsn, "seed_only_category_sizes_test")
	ctx := t.Context()

	store := &ProductSearchStore{pool: pool}
	if err := store.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, table := range []string{"brands", "colors", "base_colors", "sku_editions"} {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s after migrate: %d rows (%v), want 0", table, n, err)
		}
	}
	var sizes int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sizes`).Scan(&sizes); err != nil || sizes != len(domain.CodeSizes()) {
		t.Errorf("sizes after migrate: %d (%v), want %d", sizes, err, len(domain.CodeSizes()))
	}
	// A variant created without a size gets OS, so OS is there before any
	// reference data is loaded.
	var osName string
	if err := pool.QueryRow(ctx, `SELECT name FROM sizes WHERE code = $1`, domain.DefaultSize.Code).Scan(&osName); err != nil {
		t.Errorf("default size %s after migrate: %v", domain.DefaultSize.Code, err)
	}

	catalog := NewCatalogStore(pool)
	if _, err := catalog.CreateBrand(ctx, domain.Brand{Code: "PR", Name: "Prada"}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.DeleteBrand(ctx, "PR"); err != nil {
		t.Fatal(err)
	}
	if err := store.migrate(); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	if _, err := catalog.GetBrand(ctx, "PR"); !errors.Is(err, domain.ErrMasterNotFound) {
		t.Fatalf("deleted brand after restart: want ErrMasterNotFound, got %v", err)
	}
}
