package pg

import (
	"context"
	"errors"

	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
)

// AddWishlist implements ports.ProductWishlistStore.
func (s *ProductSearchStore) AddWishlist(ownerKey, productID string) (bool, int64, error) {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, 0, wrapDB("add wishlist begin", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`INSERT INTO product_wishlists (owner_key, product_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		ownerKey, productID,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return false, 0, ports.ErrNotFound
		}
		return false, 0, wrapDB("add wishlist insert", err)
	}
	added := tag.RowsAffected() > 0
	if added {
		if _, err := tx.Exec(ctx,
			`UPDATE products SET wishlist_count = wishlist_count + 1 WHERE id = $1`,
			productID,
		); err != nil {
			return false, 0, wrapDB("add wishlist increment", err)
		}
	}

	var count int64
	if err := tx.QueryRow(ctx,
		`SELECT wishlist_count FROM products WHERE id = $1`, productID,
	).Scan(&count); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, 0, ports.ErrNotFound
		}
		return false, 0, wrapDB("add wishlist select count", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, 0, wrapDB("add wishlist commit", err)
	}
	return added, count, nil
}

// RemoveWishlist implements ports.ProductWishlistStore.
func (s *ProductSearchStore) RemoveWishlist(ownerKey, productID string) (bool, int64, error) {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, 0, wrapDB("remove wishlist begin", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`DELETE FROM product_wishlists WHERE owner_key = $1 AND product_id = $2`,
		ownerKey, productID,
	)
	if err != nil {
		return false, 0, wrapDB("remove wishlist delete", err)
	}
	removed := tag.RowsAffected() > 0
	if removed {
		if _, err := tx.Exec(ctx,
			`UPDATE products SET wishlist_count = GREATEST(wishlist_count - 1, 0) WHERE id = $1`,
			productID,
		); err != nil {
			return false, 0, wrapDB("remove wishlist decrement", err)
		}
	}

	var count int64
	err = tx.QueryRow(ctx,
		`SELECT wishlist_count FROM products WHERE id = $1`, productID,
	).Scan(&count)
	if errors.Is(err, pgx.ErrNoRows) {
		return removed, 0, nil
	}
	if err != nil {
		return false, 0, wrapDB("remove wishlist select count", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, 0, wrapDB("remove wishlist commit", err)
	}
	return removed, count, nil
}

// ListWishlistProductIDs implements ports.ProductWishlistStore.
func (s *ProductSearchStore) ListWishlistProductIDs(ownerKey string) ([]string, error) {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT product_id FROM product_wishlists
		WHERE owner_key = $1
		ORDER BY created_at DESC, product_id ASC
	`, ownerKey)
	if err != nil {
		return nil, wrapDB("list wishlist", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, wrapDB("list wishlist scan", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapDB("list wishlist", err)
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, nil
}
