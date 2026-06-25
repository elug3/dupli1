package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
)

// migrateSchema ensures the auth service database schema is up to date.
func migrateSchema(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id       TEXT PRIMARY KEY,
			email    TEXT UNIQUE NOT NULL,
			password TEXT NOT NULL,
			roles    TEXT[] NOT NULL DEFAULT '{}'
		)`,
		// Idempotent: adds roles column to existing deployments that predate it.
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS roles TEXT[] NOT NULL DEFAULT '{}'`,
	}

	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate schema: %w", err)
		}
	}
	return nil
}
