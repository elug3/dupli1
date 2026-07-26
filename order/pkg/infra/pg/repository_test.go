package pg

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v4/pgxpool"
)

// requireDSN returns the test database DSN, skipping when Postgres is not available.
func requireDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		t.Skip("POSTGRES_URL not set; skipping postgres integration test")
	}
	return dsn
}

// freshSchema gives the test an empty schema so migrate() runs the way it does
// against a brand-new database. search_path is set on the pool config rather than
// with a SET statement, so it holds for every connection the pool opens.
func freshSchema(t *testing.T, dsn, schema string) *pgxpool.Pool {
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

// A fresh database has no payment_due_at column until the ALTER statements run, so
// anything depending on it must come after them or the service cannot start at all.
func TestMigrateOnEmptyDatabase(t *testing.T) {
	dsn := requireDSN(t)
	schema := "order_migrate_empty_test"
	pool := freshSchema(t, dsn, schema)
	repo := &Repository{pool: pool}

	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate on empty database returned error: %v", err)
	}

	ctx := context.Background()
	var columns int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'orders' AND column_name = 'payment_due_at'
	`, schema).Scan(&columns); err != nil {
		t.Fatalf("inspect orders columns: %v", err)
	}
	if columns != 1 {
		t.Fatalf("orders.payment_due_at column count = %d, want 1", columns)
	}

	var indexes int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_indexes
		WHERE schemaname = $1 AND tablename = 'orders'
		  AND indexname = 'idx_orders_pending_payment_due_at'
	`, schema).Scan(&indexes); err != nil {
		t.Fatalf("inspect orders indexes: %v", err)
	}
	if indexes != 1 {
		t.Fatalf("idx_orders_pending_payment_due_at count = %d, want 1", indexes)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_migrate_idempotent_test")
	repo := &Repository{pool: pool}

	if err := repo.migrate(); err != nil {
		t.Fatalf("first migrate returned error: %v", err)
	}
	if err := repo.migrate(); err != nil {
		t.Fatalf("second migrate returned error: %v", err)
	}
}
