package pg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
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

// loadReferenceData writes scripts/seed/reference-data.json into the schema,
// as scripts/seed-reference-data.py does for a real environment: the service
// itself seeds only the category sizes, so a test that relies on Black, OS or
// Prada existing loads them here.
func loadReferenceData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "scripts", "seed", "reference-data.json"))
	if err != nil {
		t.Fatalf("read reference data: %v", err)
	}
	var data struct {
		Brands     []domain.Brand     `json:"brands"`
		BaseColors []domain.BaseColor `json:"base_colors"`
		Colors     []domain.Color     `json:"colors"`
		Sizes      []domain.Size      `json:"sizes"`
		Editions   []domain.Edition   `json:"editions"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("parse reference data: %v", err)
	}
	ctx := t.Context()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("load reference data (%v): %v", args, err)
		}
	}
	for _, b := range data.Brands {
		exec(`INSERT INTO brands (code, name) VALUES ($1, $2) ON CONFLICT DO NOTHING`, b.Code, b.Name)
	}
	for _, b := range data.BaseColors {
		exec(`INSERT INTO base_colors (code, name, hex) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, b.Code, b.Name, b.Hex)
	}
	for _, c := range data.Colors {
		exec(`INSERT INTO colors (code, name, hex, base_code) VALUES ($1, $2, $3, NULLIF($4, '')) ON CONFLICT DO NOTHING`,
			c.Code, c.Name, c.Hex, c.BaseCode)
	}
	for _, s := range data.Sizes {
		exec(`INSERT INTO sizes (code, name) VALUES ($1, $2) ON CONFLICT DO NOTHING`, s.Code, s.Name)
	}
	for _, e := range data.Editions {
		exec(`INSERT INTO sku_editions (code, name) VALUES ($1, $2) ON CONFLICT DO NOTHING`, e.Code, e.Name)
	}
}
