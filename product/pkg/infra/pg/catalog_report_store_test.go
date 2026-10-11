package pg

import (
	"testing"
)

// Each group counts its products once however many SKUs they have, and sums
// stock across those SKUs; a SKU without a stock row adds a SKU but no units.
func TestCatalogCounts_GroupsProductsSKUsAndStock(t *testing.T) {
	dsn := requireProductDSN(t)
	pool := freshProductSchema(t, dsn, "catalog_report_test")
	ctx := t.Context()

	store := &ProductSearchStore{pool: pool}
	if err := store.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := NewInventoryStore(pool); err != nil {
		t.Fatalf("inventory migrate: %v", err)
	}
	for _, stmt := range []string{
		`INSERT INTO products (id, brand, category, sub_category, status) VALUES
			('p1', 'Prada', 'bags', 'tote', 'active'),
			('p2', 'Prada', 'bags', 'tote', 'active'),
			('p3', 'Prada', 'bags', 'tote', 'draft'),
			('p4', 'Gucci', 'clothing', '', 'active')`,
		`INSERT INTO product_variants (sku, sku_id, product_id, color, size) VALUES
			('P1_BLK', 's1', 'p1', 'BLK', 'OS'),
			('P1_RED', 's2', 'p1', 'RED', 'OS'),
			('P2_BLK', 's3', 'p2', 'BLK', 'OS')`,
		`INSERT INTO stock_items (sku_id, quantity, reserved) VALUES ('s1', 3, 1), ('s2', 4, 0)`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	counts, err := store.CatalogCounts(ctx)
	if err != nil {
		t.Fatalf("CatalogCounts: %v", err)
	}
	type key struct{ brand, cat, sub, status string }
	got := map[key][4]int{}
	for _, c := range counts {
		if c.BrandCode != "" {
			t.Fatalf("brand_code = %q, want empty for a NULL", c.BrandCode)
		}
		got[key{c.Brand, c.Category, c.SubCategory, c.Status}] = [4]int{c.Products, c.SKUs, c.Units, c.Reserved}
	}
	want := map[key][4]int{
		{"Prada", "bags", "tote", "active"}: {2, 3, 7, 1},
		{"Prada", "bags", "tote", "draft"}:  {1, 0, 0, 0},
		{"Gucci", "clothing", "", "active"}: {1, 0, 0, 0},
	}
	if len(got) != len(want) {
		t.Fatalf("groups = %v, want %v", got, want)
	}
	for k, w := range want {
		if got[k] != w {
			t.Fatalf("%v = %v, want %v", k, got[k], w)
		}
	}
}
