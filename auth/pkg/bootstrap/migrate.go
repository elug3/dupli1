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
			id                    TEXT PRIMARY KEY,
			email                 TEXT UNIQUE NOT NULL,
			password              TEXT NOT NULL,
			roles                 TEXT[]    NOT NULL DEFAULT '{}',
			is_active             BOOLEAN   NOT NULL DEFAULT TRUE,
			locked_at             TIMESTAMPTZ,
			failed_login_attempts INT       NOT NULL DEFAULT 0
		)`,
		// Idempotent column additions for existing deployments.
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS roles TEXT[] NOT NULL DEFAULT '{}'`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT TRUE`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS locked_at TIMESTAMPTZ`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS failed_login_attempts INT NOT NULL DEFAULT 0`,
	}

	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate schema: %w", err)
		}
	}
	return nil
}
