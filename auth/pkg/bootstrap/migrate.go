package bootstrap

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/lib/pq"
)

// MigrateSchema ensures the auth service database schema is up to date.
func MigrateSchema(ctx context.Context, db *sql.DB) error {
	return migrateSchema(ctx, db)
}

func migrateSchema(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id                    TEXT PRIMARY KEY,
			email                 TEXT UNIQUE NOT NULL,
			password              TEXT NOT NULL,
			permissions           TEXT[]    NOT NULL DEFAULT '{}',
			is_active             BOOLEAN   NOT NULL DEFAULT TRUE,
			locked_at             TIMESTAMPTZ,
			failed_login_attempts INT       NOT NULL DEFAULT 0,
			account_type          TEXT      NOT NULL DEFAULT 'customer'
		)`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS permissions TEXT[] NOT NULL DEFAULT '{}'`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT TRUE`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS locked_at TIMESTAMPTZ`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS failed_login_attempts INT NOT NULL DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS account_type TEXT NOT NULL DEFAULT 'customer'`,
	}

	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate schema: %w", err)
		}
	}

	if err := renameRolesColumn(ctx, db); err != nil {
		return err
	}
	if err := expandLegacyPermissionValues(ctx, db); err != nil {
		return err
	}

	backfill := []string{
		// Rename legacy account_type "admin" → "manager" (admin is a permission tier, not an account type).
		`UPDATE users SET account_type = 'manager' WHERE account_type = 'admin'`,
		`UPDATE users SET account_type = 'manager'
		 WHERE account_type = 'customer'
		   AND (permissions && ARRAY['owner','admin','user_manager','*','admin.*'])`,
		`UPDATE users SET account_type = 'service'
		 WHERE account_type = 'customer'
		   AND (permissions && ARRAY['customer_registrar','order_manager','user.create'])`,
	}
	for _, stmt := range backfill {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate schema: %w", err)
		}
	}

	profileStmts := []string{
		`CREATE TABLE IF NOT EXISTS id_sequences (
			name TEXT PRIMARY KEY,
			value BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS customer_profiles (
			user_id      TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			display_name TEXT NOT NULL DEFAULT '',
			phone        TEXT NOT NULL DEFAULT '',
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS customer_addresses (
			id              TEXT PRIMARY KEY,
			user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			label           TEXT NOT NULL DEFAULT '',
			recipient_name  TEXT NOT NULL,
			recipient_phone TEXT NOT NULL,
			postal_code     TEXT NOT NULL,
			address_line1   TEXT NOT NULL,
			address_line2   TEXT NOT NULL DEFAULT '',
			city            TEXT NOT NULL,
			province        TEXT NOT NULL,
			is_default      BOOLEAN NOT NULL DEFAULT FALSE,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_customer_addresses_user_id ON customer_addresses (user_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_customer_addresses_one_default
			ON customer_addresses (user_id) WHERE is_default`,
	}
	for _, stmt := range profileStmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate profile schema: %w", err)
		}
	}

	return nil
}

func renameRolesColumn(ctx context.Context, db *sql.DB) error {
	var rolesExists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'users'
			  AND column_name = 'roles'
		)`).Scan(&rolesExists)
	if err != nil {
		return fmt.Errorf("migrate roles column: %w", err)
	}
	if !rolesExists {
		return nil
	}

	var permissionsExists bool
	err = db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'users'
			  AND column_name = 'permissions'
		)`).Scan(&permissionsExists)
	if err != nil {
		return fmt.Errorf("migrate permissions column: %w", err)
	}

	if permissionsExists {
		if _, err := db.ExecContext(ctx, `
			UPDATE users
			   SET permissions = roles
			 WHERE cardinality(permissions) = 0
			   AND cardinality(roles) > 0`); err != nil {
			return fmt.Errorf("copy roles to permissions: %w", err)
		}
		if _, err := db.ExecContext(ctx, `ALTER TABLE users DROP COLUMN roles`); err != nil {
			return fmt.Errorf("drop roles column: %w", err)
		}
		return nil
	}

	if _, err := db.ExecContext(ctx, `ALTER TABLE users RENAME COLUMN roles TO permissions`); err != nil {
		return fmt.Errorf("rename roles to permissions: %w", err)
	}
	return nil
}

func expandLegacyPermissionValues(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT id, permissions FROM users`)
	if err != nil {
		return fmt.Errorf("list users for permission expansion: %w", err)
	}
	defer rows.Close()

	type update struct {
		id    string
		perms []string
	}
	var pending []update

	for rows.Next() {
		var id string
		var stored pq.StringArray
		if err := rows.Scan(&id, &stored); err != nil {
			return fmt.Errorf("scan user permissions: %w", err)
		}
		perms := []string(stored)
		if !permissions.NeedsExpansion(perms) {
			continue
		}
		pending = append(pending, update{
			id:    id,
			perms: permissions.ExpandLegacyRoles(perms),
		})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list users for permission expansion: %w", err)
	}

	for _, item := range pending {
		if _, err := db.ExecContext(ctx,
			`UPDATE users SET permissions = $1 WHERE id = $2`,
			pq.Array(item.perms), item.id,
		); err != nil {
			return fmt.Errorf("expand permissions for %s: %w", item.id, err)
		}
	}
	return nil
}
