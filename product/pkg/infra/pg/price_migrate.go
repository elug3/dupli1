package pg

import (
	"context"
	"fmt"
)

// migrateParentPrices moves sale/display prices from variants onto the parent
// product (source of truth). Variant price columns remain for schema compat
// but are no longer read or written by the app.
func (s *ProductSearchStore) migrateParentPrices(ctx context.Context) error {
	// Prefer min active variant price when the parent still has the default 0
	// and variants carry a non-zero price (legacy rows).
	if _, err := s.pool.Exec(ctx, `
		UPDATE products p
		SET price = v.min_price,
		    selling_price = v.selling_price
		FROM (
			SELECT DISTINCT ON (product_id)
				product_id,
				price AS min_price,
				selling_price
			FROM product_variants
			WHERE status = 'active' AND price > 0
			ORDER BY product_id, price ASC, created_at ASC
		) v
		WHERE p.id = v.product_id
		  AND (p.price = 0 OR p.price IS NULL)
	`); err != nil {
		return fmt.Errorf("backfill parent prices from variants: %w", err)
	}

	// Zero out variant prices so SKU rows are not treated as priced.
	if _, err := s.pool.Exec(ctx, `
		UPDATE product_variants
		SET price = 0, selling_price = 0
		WHERE price <> 0 OR selling_price <> 0
	`); err != nil {
		return fmt.Errorf("clear variant prices: %w", err)
	}
	return nil
}
