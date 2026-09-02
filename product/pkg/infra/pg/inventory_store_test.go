package pg

import (
	"os"
	"testing"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/jackc/pgx/v4/pgxpool"
)

func requireProductDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		t.Skip("POSTGRES_URL not set; skipping postgres integration test")
	}
	return dsn
}

func freshInventorySchema(t *testing.T, dsn, schema string) *pgxpool.Pool {
	t.Helper()
	ctx := t.Context()

	admin, err := pgxpool.Connect(ctx, withPostgresSSLMode(dsn))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer admin.Close()
	for _, stmt := range []string{
		`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`,
		`CREATE SCHEMA ` + schema,
	} {
		if _, err := admin.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare schema (%s): %v", stmt, err)
		}
	}

	cfg, err := pgxpool.ParseConfig(withPostgresSSLMode(dsn))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect with search_path: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		cleanup, err := pgxpool.Connect(ctx, withPostgresSSLMode(dsn))
		if err != nil {
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
	})
	return pool
}

// Regression for migrate seeding id_sequences when reservations already exist
// but the counter row is missing (data migration / partial reset).
func TestMigrateSeedsReservationSequenceFromExistingRows(t *testing.T) {
	dsn := requireProductDSN(t)
	pool := freshInventorySchema(t, dsn, "inv_seq_seed_test")
	ctx := t.Context()
	now := time.Now().UTC()

	if _, err := pool.Exec(ctx, `CREATE TABLE product_variants (sku_id TEXT PRIMARY KEY, sku TEXT NOT NULL)`); err != nil {
		t.Fatalf("create product_variants: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS reservations (
			id TEXT PRIMARY KEY,
			order_id TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		)`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare reservations: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO reservations (id, order_id, status, created_at, updated_at)
		VALUES ('res_000005', 'ord-1', 'held', $1, $1)
	`, now); err != nil {
		t.Fatalf("seed reservation: %v", err)
	}

	store, err := NewInventoryStore(pool)
	if err != nil {
		t.Fatalf("NewInventoryStore: %v", err)
	}

	var seq int64
	if err := pool.QueryRow(ctx, `SELECT value FROM id_sequences WHERE name = 'reservation'`).Scan(&seq); err != nil {
		t.Fatalf("read id_sequences: %v", err)
	}
	if seq != 5 {
		t.Fatalf("id_sequences value = %d, want 5", seq)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO product_variants (sku_id, sku) VALUES ('sku-1', 'TEST-1')`); err != nil {
		t.Fatalf("insert variant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO stock_items (sku_id, quantity, reserved, updated_at)
		VALUES ('sku-1', 10, 0, $1)
	`, now); err != nil {
		t.Fatalf("insert stock: %v", err)
	}

	res, err := store.CreateReservation(ctx, "ord-2", []domain.ReservationItem{{
		SkuID: "sku-1", SKU: "TEST-1", Quantity: 1,
	}}, now)
	if err != nil {
		t.Fatalf("CreateReservation: %v", err)
	}
	if res.ID != "res_000006" {
		t.Fatalf("reservation ID = %q, want res_000006", res.ID)
	}
}

// Regression for always-tracked SKUs: migrate backfills stock_items for variants
// that existed before stock tracking (missing row ⇒ available 0, not "infinite").
func TestMigrateBackfillsStockItemsForOrphanVariants(t *testing.T) {
	dsn := requireProductDSN(t)
	pool := freshInventorySchema(t, dsn, "inv_stock_backfill_test")
	ctx := t.Context()

	if _, err := pool.Exec(ctx, `CREATE TABLE product_variants (sku_id TEXT PRIMARY KEY, sku TEXT NOT NULL)`); err != nil {
		t.Fatalf("create product_variants: %v", err)
	}
	for _, skuID := range []string{"sku-with-stock", "sku-orphan"} {
		sku := skuID + "-HUMAN"
		if _, err := pool.Exec(ctx, `INSERT INTO product_variants (sku_id, sku) VALUES ($1, $2)`, skuID, sku); err != nil {
			t.Fatalf("insert variant %s: %v", skuID, err)
		}
	}
	now := time.Now().UTC()
	if _, err := pool.Exec(ctx, `
		INSERT INTO stock_items (sku_id, quantity, reserved, updated_at)
		VALUES ('sku-with-stock', 5, 1, $1)
	`, now); err != nil {
		t.Fatalf("seed existing stock row: %v", err)
	}

	store, err := NewInventoryStore(pool)
	if err != nil {
		t.Fatalf("NewInventoryStore: %v", err)
	}

	var orphanQty int
	if err := pool.QueryRow(ctx, `
		SELECT quantity FROM stock_items WHERE sku_id = 'sku-orphan'
	`).Scan(&orphanQty); err != nil {
		t.Fatalf("orphan stock row missing after migrate: %v", err)
	}
	if orphanQty != 0 {
		t.Fatalf("backfilled orphan quantity = %d, want 0", orphanQty)
	}

	existing, err := store.GetItem(ctx, "sku-with-stock")
	if err != nil {
		t.Fatalf("GetItem existing: %v", err)
	}
	if existing.Quantity != 5 || existing.Reserved != 1 {
		t.Fatalf("existing stock row mutated: %+v", existing)
	}

	orphan, err := store.GetItem(ctx, "sku-orphan")
	if err != nil {
		t.Fatalf("GetItem orphan: %v", err)
	}
	if orphan.Available() != 0 {
		t.Fatalf("orphan available = %d, want 0", orphan.Available())
	}
}

// Regression when id_sequences exists but is stale (e.g. partial DB reset).
func TestMigrateRepairsStaleReservationSequence(t *testing.T) {
	dsn := requireProductDSN(t)
	pool := freshInventorySchema(t, dsn, "inv_seq_stale_test")
	ctx := t.Context()
	now := time.Now().UTC()

	if _, err := pool.Exec(ctx, `CREATE TABLE product_variants (sku_id TEXT PRIMARY KEY, sku TEXT NOT NULL)`); err != nil {
		t.Fatalf("create product_variants: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS id_sequences (
			name TEXT PRIMARY KEY,
			value BIGINT NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS reservations (
			id TEXT PRIMARY KEY,
			order_id TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		)`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare schema: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO id_sequences (name, value) VALUES ('reservation', 0)
	`); err != nil {
		t.Fatalf("seed stale id_sequences: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO reservations (id, order_id, status, created_at, updated_at)
		VALUES ('res_000005', 'ord-1', 'held', $1, $1)
	`, now); err != nil {
		t.Fatalf("seed reservation: %v", err)
	}

	store, err := NewInventoryStore(pool)
	if err != nil {
		t.Fatalf("NewInventoryStore: %v", err)
	}

	var seq int64
	if err := pool.QueryRow(ctx, `SELECT value FROM id_sequences WHERE name = 'reservation'`).Scan(&seq); err != nil {
		t.Fatalf("read id_sequences: %v", err)
	}
	if seq != 5 {
		t.Fatalf("id_sequences value = %d, want 5 (stale row repaired)", seq)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO product_variants (sku_id, sku) VALUES ('sku-1', 'TEST-1')`); err != nil {
		t.Fatalf("insert variant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO stock_items (sku_id, quantity, reserved, updated_at)
		VALUES ('sku-1', 10, 0, $1)
	`, now); err != nil {
		t.Fatalf("insert stock: %v", err)
	}

	res, err := store.CreateReservation(ctx, "ord-2", []domain.ReservationItem{{
		SkuID: "sku-1", SKU: "TEST-1", Quantity: 1,
	}}, now)
	if err != nil {
		t.Fatalf("CreateReservation: %v", err)
	}
	if res.ID != "res_000006" {
		t.Fatalf("reservation ID = %q, want res_000006", res.ID)
	}
}

// SetQuantity must not overwrite reserved when a manager restocks during checkout.
func TestSetQuantityPreservesReserved(t *testing.T) {
	dsn := requireProductDSN(t)
	pool := freshInventorySchema(t, dsn, "inv_set_qty_test")
	ctx := t.Context()
	now := time.Now().UTC()

	if _, err := pool.Exec(ctx, `CREATE TABLE product_variants (sku_id TEXT PRIMARY KEY, sku TEXT NOT NULL)`); err != nil {
		t.Fatalf("create product_variants: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO product_variants (sku_id, sku) VALUES ('sku-1', 'TEST-1')
	`); err != nil {
		t.Fatalf("insert variant: %v", err)
	}

	store, err := NewInventoryStore(pool)
	if err != nil {
		t.Fatalf("NewInventoryStore: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO stock_items (sku_id, quantity, reserved, updated_at)
		VALUES ('sku-1', 10, 8, $1)
	`, now); err != nil {
		t.Fatalf("insert stock: %v", err)
	}

	item, err := store.SetQuantity(ctx, "sku-1", 20, now)
	if err != nil {
		t.Fatalf("SetQuantity: %v", err)
	}
	if item.Quantity != 20 || item.Reserved != 8 {
		t.Fatalf("after SetQuantity: quantity=%d reserved=%d, want 20/8", item.Quantity, item.Reserved)
	}
}
