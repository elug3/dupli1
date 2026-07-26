package pg

import (
	"context"
	"fmt"
)

// migrateParentPrices makes products.price + products.official_price the only
// pricing columns:
//  1. Ensure products.official_price exists
//  2. Backfill parent sale price from legacy variant.price when parent price is 0
//  3. Backfill parent official_price from variant/parent selling_price when still 0
//  4. Drop legacy pricing columns from products and product_variants
func (s *ProductSearchStore) migrateParentPrices(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS official_price NUMERIC(10,2) NOT NULL DEFAULT 0`,
	); err != nil {
		return fmt.Errorf("add products.official_price: %w", err)
	}

	hasVariantPrice, err := s.columnExists(ctx, "product_variants", "price")
	if err != nil {
		return err
	}
	hasVariantSelling, err := s.columnExists(ctx, "product_variants", "selling_price")
	if err != nil {
		return err
	}

	// Sale price: cheapest active variant when parent still has default 0.
	if hasVariantPrice {
		if _, err := s.pool.Exec(ctx, `
			UPDATE products p
			SET price = v.min_price
			FROM (
				SELECT product_id, MIN(price) AS min_price
				FROM product_variants
				WHERE status = 'active' AND price > 0
				GROUP BY product_id
			) v
			WHERE p.id = v.product_id
			  AND p.price = 0
		`); err != nil {
			return fmt.Errorf("backfill parent price from variants: %w", err)
		}
	}

	// Official / list price: highest active variant selling_price when parent
	// official is still 0 — independent of whether sale price was already set.
	if hasVariantSelling {
		if _, err := s.pool.Exec(ctx, `
			UPDATE products p
			SET official_price = v.max_selling
			FROM (
				SELECT product_id, MAX(selling_price) AS max_selling
				FROM product_variants
				WHERE status = 'active' AND selling_price > 0
				GROUP BY product_id
			) v
			WHERE p.id = v.product_id
			  AND p.official_price = 0
		`); err != nil {
			return fmt.Errorf("backfill parent official_price from variants: %w", err)
		}
	}

	hasProductSelling, err := s.columnExists(ctx, "products", "selling_price")
	if err != nil {
		return err
	}
	if hasProductSelling {
		if _, err := s.pool.Exec(ctx, `
			UPDATE products
			SET official_price = selling_price
			WHERE official_price = 0 AND selling_price > 0
		`); err != nil {
			return fmt.Errorf("copy products.selling_price to official_price: %w", err)
		}
		if _, err := s.pool.Exec(ctx,
			`ALTER TABLE products DROP COLUMN IF EXISTS selling_price`,
		); err != nil {
			return fmt.Errorf("drop products.selling_price: %w", err)
		}
	}

	if hasVariantPrice || hasVariantSelling {
		if _, err := s.pool.Exec(ctx,
			`ALTER TABLE product_variants DROP COLUMN IF EXISTS selling_price`,
		); err != nil {
			return fmt.Errorf("drop product_variants.selling_price: %w", err)
		}
		if _, err := s.pool.Exec(ctx,
			`ALTER TABLE product_variants DROP COLUMN IF EXISTS price`,
		); err != nil {
			return fmt.Errorf("drop product_variants.price: %w", err)
		}
	}
	return nil
}

func (s *ProductSearchStore) columnExists(ctx context.Context, table, column string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = $1
			  AND column_name = $2
		)
	`, table, column).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check column %s.%s: %w", table, column, err)
	}
	return exists, nil
}
