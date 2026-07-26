package pg

import (
	"context"
	"fmt"
)

// migrateParentPrices makes products.price + products.official_price the only
// pricing columns:
//  1. Ensure products.official_price exists
//  2. Backfill parent prices from legacy variant columns when present
//  3. Copy products.selling_price → official_price when needed
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

	// Prefer min active variant price when the parent still has the default 0
	// and variants carry a non-zero price (legacy rows).
	if hasVariantPrice {
		sellingExpr := "0::numeric"
		if hasVariantSelling {
			sellingExpr = "selling_price"
		}
		sql := fmt.Sprintf(`
			UPDATE products p
			SET price = v.min_price,
			    official_price = CASE
			      WHEN p.official_price = 0 THEN v.selling_price
			      ELSE p.official_price
			    END
			FROM (
				SELECT DISTINCT ON (product_id)
					product_id,
					price AS min_price,
					%s AS selling_price
				FROM product_variants
				WHERE status = 'active' AND price > 0
				ORDER BY product_id, price ASC, created_at ASC
			) v
			WHERE p.id = v.product_id
			  AND (p.price = 0 OR p.price IS NULL)
		`, sellingExpr)
		if _, err := s.pool.Exec(ctx, sql); err != nil {
			return fmt.Errorf("backfill parent prices from variants: %w", err)
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
