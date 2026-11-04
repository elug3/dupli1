package pg

import (
	"context"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/shared/pkg/money"
	"github.com/jackc/pgx/v4/pgxpool"
)

// wonPriceFixture is a migrated schema holding one parent and two SKUs: one
// inheriting the parent's prices and one with its own overrides.
type wonPriceFixture struct {
	store              *ProductSearchStore
	pool               *pgxpool.Pool
	parentID           string
	inheritSKU, ownSKU string
}

func newWonPriceFixture(t *testing.T, schema string) wonPriceFixture {
	t.Helper()
	ctx := t.Context()
	pool := freshProductSchema(t, requireProductDSN(t), schema)
	store := &ProductSearchStore{pool: pool}
	if err := store.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := NewInventoryStore(pool); err != nil {
		t.Fatalf("inventory migrate: %v", err)
	}
	for _, stmt := range []string{
		`INSERT INTO brands (code, name) VALUES ('WON', 'Won Test') ON CONFLICT DO NOTHING`,
		`INSERT INTO styles (brand_code, code, name) VALUES ('WON', 'BAG01', 'Won Bag') ON CONFLICT DO NOTHING`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("seed masters: %v", err)
		}
	}
	parent, err := store.CreateProduct(ctx, domain.Product{
		Name: "Won Bag", BrandCode: "WON", StyleCode: "BAG01", Category: "bags",
		Price: 300000, OfficialPrice: 390000, Status: "active",
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	own, official := money.Won(380000), money.Won(450000)
	inherit, err := store.CreateVariant(ctx, domain.Variant{ProductID: parent.ID, ColorCode: "BLK", SizeCode: "OS", Status: "active"})
	if err != nil {
		t.Fatalf("create inheriting variant: %v", err)
	}
	priced, err := store.CreateVariant(ctx, domain.Variant{
		ProductID: parent.ID, ColorCode: "WHT", SizeCode: "OS", Status: "active",
		PriceOverride: &own, OfficialPriceOverride: &official,
	})
	if err != nil {
		t.Fatalf("create priced variant: %v", err)
	}
	return wonPriceFixture{store: store, pool: pool, parentID: parent.ID, inheritSKU: inherit.SKU, ownSKU: priced.SKU}
}

func (f wonPriceFixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// legacyPrices reads what code from before the BIGINT columns would see.
func (f wonPriceFixture) legacyPrices(t *testing.T) (price, official string, override, officialOverride *string) {
	t.Helper()
	ctx := context.Background()
	if err := f.pool.QueryRow(ctx,
		`SELECT price::TEXT, official_price::TEXT FROM products WHERE id = $1`, f.parentID,
	).Scan(&price, &official); err != nil {
		t.Fatalf("read legacy parent prices: %v", err)
	}
	if err := f.pool.QueryRow(ctx,
		`SELECT price_won::TEXT, official_price_won::TEXT FROM product_variants WHERE sku = $1`, f.ownSKU,
	).Scan(&override, &officialOverride); err != nil {
		t.Fatalf("read legacy overrides: %v", err)
	}
	return price, official, override, officialOverride
}

func (f wonPriceFixture) assertPrices(t *testing.T, wantPrice, wantOfficial money.Won, wantOverride, wantOfficialOverride *money.Won) {
	t.Helper()
	ctx := context.Background()
	p, err := f.store.GetProduct(ctx, f.parentID)
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	if p.Price != wantPrice || p.OfficialPrice != wantOfficial {
		t.Fatalf("parent = %d/%d, want %d/%d", p.Price, p.OfficialPrice, wantPrice, wantOfficial)
	}
	v, err := f.store.GetVariant(ctx, f.ownSKU)
	if err != nil {
		t.Fatalf("get variant: %v", err)
	}
	if !sameWon(v.PriceOverride, wantOverride) || !sameWon(v.OfficialPriceOverride, wantOfficialOverride) {
		t.Fatalf("overrides = %v/%v, want %v/%v", fmtWon(v.PriceOverride), fmtWon(v.OfficialPriceOverride),
			fmtWon(wantOverride), fmtWon(wantOfficialOverride))
	}
	inherit, err := f.store.GetVariant(ctx, f.inheritSKU)
	if err != nil {
		t.Fatalf("get inheriting variant: %v", err)
	}
	if inherit.PriceOverride != nil || inherit.OfficialPriceOverride != nil {
		t.Fatalf("inheriting SKU gained an override: %v/%v", fmtWon(inherit.PriceOverride), fmtWon(inherit.OfficialPriceOverride))
	}
}

func sameWon(a, b *money.Won) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func fmtWon(w *money.Won) any {
	if w == nil {
		return "nil"
	}
	return int64(*w)
}

func won(v int64) *money.Won {
	w := money.Won(v)
	return &w
}

// Writes keep the NUMERIC columns that code from before this release reads,
// so a rollback finds every price current.
func TestWonPrices_WritesKeepLegacyColumns(t *testing.T) {
	f := newWonPriceFixture(t, "won_price_dual_write")
	ctx := t.Context()
	if !f.store.legacyPrices {
		t.Fatal("a schema with NUMERIC price columns must run in legacy mode")
	}

	price, official, override, officialOverride := f.legacyPrices(t)
	if price != "300000.00" || official != "390000.00" || override == nil || *override != "380000.00" ||
		officialOverride == nil || *officialOverride != "450000.00" {
		t.Fatalf("created rows: legacy columns = %s/%s/%v/%v", price, official, override, officialOverride)
	}

	p, _ := f.store.GetProduct(ctx, f.parentID)
	p.Price, p.OfficialPrice = 310000, 400000
	if _, err := f.store.UpdateProduct(ctx, *p); err != nil {
		t.Fatalf("update product: %v", err)
	}
	v, _ := f.store.GetVariant(ctx, f.ownSKU)
	v.PriceOverride, v.OfficialPriceOverride = nil, won(460000)
	if _, err := f.store.UpdateVariant(ctx, *v); err != nil {
		t.Fatalf("update variant: %v", err)
	}

	price, official, override, officialOverride = f.legacyPrices(t)
	if price != "310000.00" || official != "400000.00" || override != nil ||
		officialOverride == nil || *officialOverride != "460000.00" {
		t.Fatalf("updated rows: legacy columns = %s/%s/%v/%v", price, official, override, officialOverride)
	}
	f.assertPrices(t, 310000, 400000, nil, won(460000))
}

// An upgrade copies NUMERIC prices into the BIGINT columns, rounding a
// fractional legacy value half away from zero, as cart and order already did.
func TestWonPrices_UpgradeCopiesLegacyValues(t *testing.T) {
	f := newWonPriceFixture(t, "won_price_upgrade")

	// The database as code from before this release left it: NUMERIC only,
	// with fractional values the old float64 path accepted.
	for _, stmt := range []string{
		`ALTER TABLE products DROP COLUMN price_won, DROP COLUMN official_price_won`,
		`ALTER TABLE product_variants DROP COLUMN price_override_won, DROP COLUMN official_price_override_won`,
		`UPDATE products SET price = 1234.50, official_price = 999.49`,
		`UPDATE product_variants SET price_won = 380000.50, official_price_won = NULL WHERE price_won IS NOT NULL`,
	} {
		f.exec(t, stmt)
	}

	if err := f.store.migrate(); err != nil {
		t.Fatalf("upgrade migrate: %v", err)
	}
	f.assertPrices(t, 1235, 999, won(380001), nil)
}

// Rows written by older code after a rollback are picked up on the next start.
func TestWonPrices_StartupResyncsAfterRollback(t *testing.T) {
	f := newWonPriceFixture(t, "won_price_resync")

	// What older code does: write only the NUMERIC columns.
	f.exec(t, `UPDATE products SET price = 250000, official_price = 0 WHERE id = $1`, f.parentID)
	f.exec(t, `UPDATE product_variants SET price_won = NULL, official_price_won = 470000 WHERE sku = $1`, f.ownSKU)

	if err := f.store.migrate(); err != nil {
		t.Fatalf("migrate after rollback: %v", err)
	}
	f.assertPrices(t, 250000, 0, nil, won(470000))

	// Nothing to copy on the next start.
	tag, err := f.pool.Exec(t.Context(), `UPDATE products SET price_won = price_won WHERE price_won <> ROUND(price)::BIGINT`)
	if err != nil || tag.RowsAffected() != 0 {
		t.Fatalf("columns out of step after resync: rows=%d err=%v", tag.RowsAffected(), err)
	}
}

// Once the NUMERIC columns are dropped, a rollback to this release must run on
// the BIGINT columns alone, not copy zeros from the NUMERIC columns its own
// migration re-adds.
func TestWonPrices_RunsWithoutLegacyColumns(t *testing.T) {
	f := newWonPriceFixture(t, "won_price_no_legacy")
	ctx := t.Context()

	f.exec(t, `ALTER TABLE products DROP COLUMN price, DROP COLUMN official_price`)
	f.exec(t, `ALTER TABLE product_variants DROP COLUMN price_won, DROP COLUMN official_price_won`)
	if err := f.store.migrate(); err != nil {
		t.Fatalf("migrate without legacy columns: %v", err)
	}
	if f.store.legacyPrices {
		t.Fatal("legacy mode must stay off once products.price is gone")
	}
	f.assertPrices(t, 300000, 390000, won(380000), won(450000))

	p, _ := f.store.GetProduct(ctx, f.parentID)
	p.Price = 320000
	if _, err := f.store.UpdateProduct(ctx, *p); err != nil {
		t.Fatalf("update product: %v", err)
	}
	v, _ := f.store.GetVariant(ctx, f.ownSKU)
	v.PriceOverride = won(390000)
	if _, err := f.store.UpdateVariant(ctx, *v); err != nil {
		t.Fatalf("update variant: %v", err)
	}
	if _, err := f.store.CreateVariant(ctx, domain.Variant{
		ProductID: f.parentID, ColorCode: "CRM", SizeCode: "OS", Status: "active", PriceOverride: won(395000),
	}); err != nil {
		t.Fatalf("create variant: %v", err)
	}
	f.assertPrices(t, 320000, 390000, won(390000), won(450000))
}

// sort=price reads the BIGINT columns: the lowest active effective price,
// override or inherited.
func TestWonPrices_SortByPrice(t *testing.T) {
	f := newWonPriceFixture(t, "won_price_sort")
	ctx := t.Context()
	f.exec(t, `INSERT INTO styles (brand_code, code, name) VALUES ('WON', 'BAG02', 'Cheaper Bag')`)
	cheaper, err := f.store.CreateProduct(ctx, domain.Product{
		Name: "Cheaper Bag", BrandCode: "WON", StyleCode: "BAG02", Category: "bags",
		Price: 200000, Status: "active",
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	if _, err := f.store.CreateVariant(ctx, domain.Variant{ProductID: cheaper.ID, ColorCode: "BLK", SizeCode: "OS", Status: "active"}); err != nil {
		t.Fatalf("create variant: %v", err)
	}

	for _, tc := range []struct {
		order string
		first string
	}{{"asc", cheaper.ID}, {"desc", f.parentID}} {
		got, _, err := f.store.SearchProducts(ctx, map[string]string{"sort": "price", "order": tc.order})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(got) != 2 || got[0].ID != tc.first {
			t.Fatalf("sort=price&order=%s: first = %v, want %s", tc.order, got, tc.first)
		}
	}
}
