package pg

import (
	"testing"

	"github.com/jackc/pgx/v4/pgxpool"
)

// freshProductSchema returns a pool on a new, empty schema of the test
// database, dropped again when the test ends.
func freshProductSchema(t *testing.T, dsn, schema string) *pgxpool.Pool {
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
