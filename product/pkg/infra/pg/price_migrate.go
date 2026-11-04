package pg

import (
	"context"
	"fmt"
	"log"

	"github.com/elug3/dupli1/shared/pkg/money"
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

// migrateWonPrices moves prices onto whole-won BIGINT columns (step 1 of
// docs/product-api-v2-plan.md):
//
//	products.price                      NUMERIC(10,2) → products.price_won                            BIGINT NOT NULL
//	products.official_price             NUMERIC(10,2) → products.official_price_won                   BIGINT NOT NULL
//	product_variants.price_won          NUMERIC(10,2) → product_variants.price_override_won           BIGINT, NULL = inherit
//	product_variants.official_price_won NUMERIC(10,2) → product_variants.official_price_override_won  BIGINT, NULL = inherit
//
// Inline migrations can't make breaking changes, so this release is
// additive: the store reads the BIGINT columns and, while the NUMERIC ones
// exist, writes both. Code from before this release reads and writes only the
// NUMERIC columns, so a rollback finds them current; and every start copies
// NUMERIC → BIGINT for any row such code wrote in the meantime, rounding half
// away from zero the way money.FromProductPrice already rounds for cart and
// order. The NUMERIC columns are dropped a release later.
//
// Legacy mode needs products.price, which only CREATE TABLE makes. A rollback
// to this release after the drop therefore runs on the BIGINT columns alone,
// instead of copying zeros out of NUMERIC columns that migrate() re-adds.
func (s *ProductSearchStore) migrateWonPrices(ctx context.Context) error {
	for _, stmt := range []string{
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS price_won BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS official_price_won BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE product_variants ADD COLUMN IF NOT EXISTS price_override_won BIGINT`,
		`ALTER TABLE product_variants ADD COLUMN IF NOT EXISTS official_price_override_won BIGINT`,
	} {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migrate won prices: %s: %w", stmt, err)
		}
	}

	s.legacyPrices = true
	for _, col := range [][2]string{
		{"products", "price"},
		{"products", "official_price"},
		{"product_variants", "price_won"},
		{"product_variants", "official_price_won"},
	} {
		exists, err := s.columnExists(ctx, col[0], col[1])
		if err != nil {
			return err
		}
		if !exists {
			s.legacyPrices = false
			return nil
		}
	}

	parents, err := s.pool.Exec(ctx, `
		UPDATE products
		   SET price_won = ROUND(price)::BIGINT,
		       official_price_won = ROUND(official_price)::BIGINT
		 WHERE price_won <> ROUND(price)::BIGINT
		    OR official_price_won <> ROUND(official_price)::BIGINT
	`)
	if err != nil {
		return fmt.Errorf("copy products NUMERIC prices to BIGINT: %w", err)
	}
	variants, err := s.pool.Exec(ctx, `
		UPDATE product_variants
		   SET price_override_won = ROUND(price_won)::BIGINT,
		       official_price_override_won = ROUND(official_price_won)::BIGINT
		 WHERE price_override_won IS DISTINCT FROM ROUND(price_won)::BIGINT
		    OR official_price_override_won IS DISTINCT FROM ROUND(official_price_won)::BIGINT
	`)
	if err != nil {
		return fmt.Errorf("copy product_variants NUMERIC prices to BIGINT: %w", err)
	}
	if n := parents.RowsAffected() + variants.RowsAffected(); n > 0 {
		log.Printf("product migrate: copied %d products and %d variants from NUMERIC to BIGINT prices",
			parents.RowsAffected(), variants.RowsAffected())
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

// wonArg passes an optional price to Postgres: nil is NULL (inherit).
func wonArg(w *money.Won) *int64 {
	if w == nil {
		return nil
	}
	v := int64(*w)
	return &v
}

// wonFromNullable reads an optional BIGINT price back; NULL is nil.
func wonFromNullable(v *int64) *money.Won {
	if v == nil {
		return nil
	}
	w := money.Won(*v)
	return &w
}
