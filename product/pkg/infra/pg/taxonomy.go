package pg

import (
	"context"
	"fmt"

	"github.com/elug3/dupli1/product/pkg/domain"
)

// migrateProductTaxonomy adds bag merchandising columns and seed master tables
// for subcategory / bag style / target (storefront filters; not SKU segments).
func (s *ProductSearchStore) migrateProductTaxonomy(ctx context.Context) error {
	for _, col := range []struct{ name, def string }{
		{"sub_category", "TEXT NOT NULL DEFAULT ''"},
		{"bag_style", "TEXT NOT NULL DEFAULT ''"},
		{"target", "TEXT NOT NULL DEFAULT ''"},
	} {
		if _, err := s.pool.Exec(ctx, fmt.Sprintf(
			"ALTER TABLE products ADD COLUMN IF NOT EXISTS %s %s", col.name, col.def,
		)); err != nil {
			return fmt.Errorf("migrate products add column %s: %w", col.name, err)
		}
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS bag_subcategories (
			code TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS bag_styles (
			code TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS bag_targets (
			code TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0
		)`,
		`ALTER TABLE bag_subcategories ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE bag_styles ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE bag_targets ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 0`,
		`CREATE INDEX IF NOT EXISTS idx_products_sub_category ON products(sub_category)`,
		`CREATE INDEX IF NOT EXISTS idx_products_bag_style ON products(bag_style)`,
		`CREATE INDEX IF NOT EXISTS idx_products_target ON products(target)`,
	}
	for _, q := range statements {
		if _, err := s.pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate product taxonomy: %w", err)
		}
	}

	if err := seedCatalogTerms(ctx, s, "bag_subcategories", domain.SeedSubCategories); err != nil {
		return err
	}
	if err := seedCatalogTerms(ctx, s, "bag_styles", domain.SeedBagStyles); err != nil {
		return err
	}
	if err := seedCatalogTerms(ctx, s, "bag_targets", domain.SeedTargets); err != nil {
		return err
	}
	return nil
}

func seedCatalogTerms(ctx context.Context, s *ProductSearchStore, table string, terms []domain.CatalogTerm) error {
	for i, t := range terms {
		if _, err := s.pool.Exec(ctx,
			fmt.Sprintf(`INSERT INTO %s (code, name, sort_order) VALUES ($1, $2, $3)
			 ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, sort_order = EXCLUDED.sort_order`, table),
			t.Code, t.Name, i,
		); err != nil {
			return fmt.Errorf("seed %s: %w", table, err)
		}
	}
	return nil
}
