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

// freshSchema gives each test an empty schema so migrate() runs the same way it
// does against a brand-new database.
func freshSchema(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.Connect(ctx, withPostgresSSLMode(dsn))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	schema := "order_migrate_test"
	for _, stmt := range []string{
		`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`,
		`CREATE SCHEMA ` + schema,
		`SET search_path TO ` + schema,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			pool.Close()
			t.Fatalf("prepare schema (%s): %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
		pool.Close()
	})
	return pool
}

// A fresh database has no payment_due_at column until the ALTER statements run, so
// anything depending on it must come after them or the service cannot start at all.
func TestMigrateOnEmptyDatabase(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn)
	repo := &Repository{pool: pool}

	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate on empty database returned error: %v", err)
	}

	ctx := context.Background()
	var columns int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'orders' AND column_name = 'payment_due_at'
	`).Scan(&columns); err != nil {
		t.Fatalf("inspect orders columns: %v", err)
	}
	if columns != 1 {
		t.Fatalf("orders.payment_due_at column count = %d, want 1", columns)
	}

	var indexes int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_indexes
		WHERE tablename = 'orders' AND indexname = 'idx_orders_pending_payment_due_at'
	`).Scan(&indexes); err != nil {
		t.Fatalf("inspect orders indexes: %v", err)
	}
	if indexes != 1 {
		t.Fatalf("idx_orders_pending_payment_due_at count = %d, want 1", indexes)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn)
	repo := &Repository{pool: pool}

	if err := repo.migrate(); err != nil {
		t.Fatalf("first migrate returned error: %v", err)
	}
	if err := repo.migrate(); err != nil {
		t.Fatalf("second migrate returned error: %v", err)
	}
}
