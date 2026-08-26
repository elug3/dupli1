package pg

import (
	"context"
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
	ctx := context.Background()

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
	ctx := context.Background()
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
