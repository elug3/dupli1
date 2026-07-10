package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/elug3/dupli1/cart/pkg/domain"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(connString string) (*Repository, error) {
	connString = withPostgresSSLMode(connString)
	pool, err := pgxpool.Connect(context.Background(), connString)
	if err != nil {
		return nil, fmt.Errorf("connect cart database: %w", err)
	}

	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		pool.Close()
		return nil, err
	}
	return repo, nil
}

func (r *Repository) Close() {
	if r.pool != nil {
		r.pool.Close()
	}
}

func (r *Repository) migrate() error {
	ctx := context.Background()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS carts (
			customer_id TEXT PRIMARY KEY,
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS cart_items (
			customer_id TEXT NOT NULL REFERENCES carts(customer_id) ON DELETE CASCADE,
			sku         TEXT NOT NULL,
			quantity    INTEGER NOT NULL CHECK (quantity > 0),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (customer_id, sku)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := r.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migrate cart schema: %w", err)
		}
	}
	if _, err := r.pool.Exec(ctx, `ALTER TABLE cart_items ADD COLUMN IF NOT EXISTS sku_id TEXT`); err != nil {
		return fmt.Errorf("migrate cart schema: %w", err)
	}
	if err := r.promoteSkuIDPrimaryKey(ctx); err != nil {
		return err
	}
	return nil
}

// promoteSkuIDPrimaryKey swaps cart_items' primary key from (customer_id, sku)
// to (customer_id, sku_id). Carts are ephemeral working state, not historical
// records, so any row still missing sku_id (written before the service
// started resolving/persisting it) is simply purged rather than backfilled —
// the customer can just re-add the item. Safe to run on every startup: it
// checks the current primary key columns first and does nothing once already
// promoted.
func (r *Repository) promoteSkuIDPrimaryKey(ctx context.Context) error {
	var pkColumns []string
	rows, err := r.pool.Query(ctx, `
		SELECT a.attname
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = 'cart_items'::regclass AND i.indisprimary
		ORDER BY array_position(i.indkey, a.attnum)
	`)
	if err != nil {
		return fmt.Errorf("check cart_items primary key: %w", err)
	}
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			rows.Close()
			return fmt.Errorf("check cart_items primary key: %w", err)
		}
		pkColumns = append(pkColumns, col)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("check cart_items primary key: %w", err)
	}
	if len(pkColumns) == 2 && pkColumns[1] == "sku_id" {
		return nil
	}

	if _, err := r.pool.Exec(ctx, `DELETE FROM cart_items WHERE sku_id IS NULL`); err != nil {
		return fmt.Errorf("purge stale cart_items missing sku_id: %w", err)
	}
	if _, err := r.pool.Exec(ctx, `ALTER TABLE cart_items ALTER COLUMN sku_id SET NOT NULL`); err != nil {
		return fmt.Errorf("set cart_items.sku_id not null: %w", err)
	}
	if _, err := r.pool.Exec(ctx, `ALTER TABLE cart_items DROP CONSTRAINT cart_items_pkey`); err != nil {
		return fmt.Errorf("drop legacy cart_items pkey: %w", err)
	}
	if _, err := r.pool.Exec(ctx, `ALTER TABLE cart_items ADD PRIMARY KEY (customer_id, sku_id)`); err != nil {
		return fmt.Errorf("promote cart_items primary key: %w", err)
	}
	return nil
}

func (r *Repository) GetItems(ctx context.Context, customerID string) ([]domain.StoredItem, time.Time, error) {
	if err := ctx.Err(); err != nil {
		return nil, time.Time{}, err
	}

	var updatedAt time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT updated_at FROM carts WHERE customer_id = $1`, customerID,
	).Scan(&updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return []domain.StoredItem{}, time.Time{}, nil
	}
	if err != nil {
		return nil, time.Time{}, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT sku, quantity, COALESCE(sku_id, '') FROM cart_items WHERE customer_id = $1 ORDER BY sku`, customerID,
	)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer rows.Close()

	var items []domain.StoredItem
	for rows.Next() {
		var item domain.StoredItem
		if err := rows.Scan(&item.SKU, &item.Quantity, &item.SkuID); err != nil {
			return nil, time.Time{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, time.Time{}, err
	}
	if items == nil {
		items = []domain.StoredItem{}
	}
	return items, updatedAt, nil
}

func (r *Repository) ReplaceItems(ctx context.Context, customerID string, items []domain.StoredItem, updatedAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`INSERT INTO carts (customer_id, updated_at) VALUES ($1, $2)
		 ON CONFLICT (customer_id) DO UPDATE SET updated_at = EXCLUDED.updated_at`,
		customerID, updatedAt,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM cart_items WHERE customer_id = $1`, customerID); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := tx.Exec(ctx,
			`INSERT INTO cart_items (customer_id, sku, sku_id, quantity, updated_at) VALUES ($1, $2, $3, $4, $5)`,
			customerID, item.SKU, nullIfEmpty(item.SkuID), item.Quantity, updatedAt,
		); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) UpsertItem(ctx context.Context, customerID string, item domain.StoredItem, updatedAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`INSERT INTO carts (customer_id, updated_at) VALUES ($1, $2)
		 ON CONFLICT (customer_id) DO UPDATE SET updated_at = EXCLUDED.updated_at`,
		customerID, updatedAt,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO cart_items (customer_id, sku, sku_id, quantity, updated_at) VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (customer_id, sku_id) DO UPDATE SET sku = EXCLUDED.sku, quantity = EXCLUDED.quantity, updated_at = EXCLUDED.updated_at`,
		customerID, item.SKU, nullIfEmpty(item.SkuID), item.Quantity, updatedAt,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) RemoveItem(ctx context.Context, customerID, sku string, updatedAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE carts SET updated_at = $2 WHERE customer_id = $1`, customerID, updatedAt,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM cart_items WHERE customer_id = $1 AND sku = $2`, customerID, sku,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) RemoveItemBySkuID(ctx context.Context, customerID, skuID string, updatedAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE carts SET updated_at = $2 WHERE customer_id = $1`, customerID, updatedAt,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM cart_items WHERE customer_id = $1 AND sku_id = $2`, customerID, skuID,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (r *Repository) Clear(ctx context.Context, customerID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM cart_items WHERE customer_id = $1`, customerID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM carts WHERE customer_id = $1`, customerID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
