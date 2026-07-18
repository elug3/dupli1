package pg

import (
	"context"
	"fmt"
)

// RecordUniqueView inserts a unique (guest, product) view and increments products.view_count
// when the view is new. Implements ports.ProductViewStore.
func (s *ProductSearchStore) RecordUniqueView(guestID, productID string) (bool, error) {
	if guestID == "" || productID == "" {
		return false, fmt.Errorf("guest id and product id are required")
	}
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, wrapDB("record unique view begin", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`INSERT INTO product_views (guest_id, product_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		guestID, productID,
	)
	if err != nil {
		return false, wrapDB("record unique view insert", err)
	}
	if tag.RowsAffected() == 0 {
		if err := tx.Commit(ctx); err != nil {
			return false, wrapDB("record unique view commit", err)
		}
		return false, nil
	}

	if _, err := tx.Exec(ctx,
		`UPDATE products SET view_count = view_count + 1 WHERE id = $1`,
		productID,
	); err != nil {
		return false, wrapDB("record unique view increment", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, wrapDB("record unique view commit", err)
	}
	return true, nil
}
