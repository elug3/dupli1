package pg

import (
	"context"
	"fmt"

	"github.com/elug3/dupli1/product/pkg/domain"
)

// migrateSKUMasters creates brand/color/size/edition/styles master tables, adds
// segment code columns, seeds well-known rows, backfills products/styles, and
// adds RESTRICT foreign keys (Phase A).
func (s *ProductSearchStore) migrateSKUMasters(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS brands (
			id   BIGSERIAL PRIMARY KEY,
			code TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS colors (
			id   BIGSERIAL PRIMARY KEY,
			code TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS sizes (
			id   BIGSERIAL PRIMARY KEY,
			code TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS sku_editions (
			id   BIGSERIAL PRIMARY KEY,
			code TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS subcategories (
			id   BIGSERIAL PRIMARY KEY,
			code TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS occasions (
			id   BIGSERIAL PRIMARY KEY,
			code TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS targets (
			id   BIGSERIAL PRIMARY KEY,
			code TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL DEFAULT ''
		)`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS brand_code TEXT`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS style_code TEXT`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS subcategory_code TEXT`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS occasion_code TEXT`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS target_code TEXT`,
		`ALTER TABLE product_variants ADD COLUMN IF NOT EXISTS color_code TEXT`,
		`ALTER TABLE product_variants ADD COLUMN IF NOT EXISTS edition_code TEXT`,
		`ALTER TABLE product_variants ADD COLUMN IF NOT EXISTS size_code TEXT`,
	}
	for _, q := range statements {
		if _, err := s.pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate sku masters: %w", err)
		}
	}

	// Drop NOT NULL / default '' so empty becomes NULL (required for FKs).
	for _, q := range []string{
		`ALTER TABLE products ALTER COLUMN brand_code DROP NOT NULL`,
		`ALTER TABLE products ALTER COLUMN brand_code DROP DEFAULT`,
		`ALTER TABLE products ALTER COLUMN style_code DROP NOT NULL`,
		`ALTER TABLE products ALTER COLUMN style_code DROP DEFAULT`,
		`ALTER TABLE products ALTER COLUMN subcategory_code DROP NOT NULL`,
		`ALTER TABLE products ALTER COLUMN subcategory_code DROP DEFAULT`,
		`ALTER TABLE products ALTER COLUMN occasion_code DROP NOT NULL`,
		`ALTER TABLE products ALTER COLUMN occasion_code DROP DEFAULT`,
		`ALTER TABLE products ALTER COLUMN target_code DROP NOT NULL`,
		`ALTER TABLE products ALTER COLUMN target_code DROP DEFAULT`,
		`ALTER TABLE product_variants ALTER COLUMN color_code DROP NOT NULL`,
		`ALTER TABLE product_variants ALTER COLUMN color_code DROP DEFAULT`,
		`ALTER TABLE product_variants ALTER COLUMN size_code DROP NOT NULL`,
		`ALTER TABLE product_variants ALTER COLUMN size_code DROP DEFAULT`,
		`ALTER TABLE product_variants ALTER COLUMN edition_code DROP NOT NULL`,
		`ALTER TABLE product_variants ALTER COLUMN edition_code DROP DEFAULT`,
		`UPDATE products SET brand_code = NULL WHERE brand_code = ''`,
		`UPDATE products SET style_code = NULL WHERE style_code = ''`,
		`UPDATE products SET subcategory_code = NULL WHERE subcategory_code = ''`,
		`UPDATE products SET occasion_code = NULL WHERE occasion_code = ''`,
		`UPDATE products SET target_code = NULL WHERE target_code = ''`,
		`UPDATE product_variants SET color_code = NULL WHERE color_code = ''`,
		`UPDATE product_variants SET size_code = NULL WHERE size_code = ''`,
		`UPDATE product_variants SET edition_code = NULL WHERE edition_code = ''`,
	} {
		s.pool.Exec(ctx, q)
	}

	if err := s.seedSKUMasters(ctx); err != nil {
		return err
	}
	if err := s.backfillProductSKUCodes(ctx); err != nil {
		return err
	}
	if err := s.migrateStylesAndFKs(ctx); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `
		CREATE UNIQUE INDEX IF NOT EXISTS ux_products_brand_style
		ON products (brand_code, style_code)
		WHERE brand_code IS NOT NULL AND style_code IS NOT NULL
	`); err != nil {
		return fmt.Errorf("create brand/style unique index: %w", err)
	}
	return nil
}

func (s *ProductSearchStore) migrateStylesAndFKs(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS styles (
			brand_code TEXT NOT NULL REFERENCES brands(code) ON DELETE RESTRICT,
			code       TEXT NOT NULL,
			name       TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (brand_code, code)
		)
	`); err != nil {
		return fmt.Errorf("create styles: %w", err)
	}

	// Backfill styles from products that already have codes.
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO styles (brand_code, code, name)
		SELECT DISTINCT ON (p.brand_code, p.style_code)
			p.brand_code, p.style_code,
			COALESCE(NULLIF(p.name, ''), p.style_code)
		FROM products p
		WHERE p.brand_code IS NOT NULL AND p.style_code IS NOT NULL
		  AND EXISTS (SELECT 1 FROM brands b WHERE b.code = p.brand_code)
		ON CONFLICT (brand_code, code) DO NOTHING
	`); err != nil {
		return fmt.Errorf("backfill styles: %w", err)
	}

	// Backfill variant color/size codes from display names where missing.
	if err := s.backfillVariantSegmentCodes(ctx); err != nil {
		return err
	}

	fks := []struct{ name, sql string }{
		{"products_brand_fk", `
			ALTER TABLE products
			ADD CONSTRAINT products_brand_fk
			FOREIGN KEY (brand_code) REFERENCES brands(code) ON DELETE RESTRICT`},
		{"products_style_fk", `
			ALTER TABLE products
			ADD CONSTRAINT products_style_fk
			FOREIGN KEY (brand_code, style_code) REFERENCES styles(brand_code, code) ON DELETE RESTRICT`},
		{"variants_color_fk", `
			ALTER TABLE product_variants
			ADD CONSTRAINT variants_color_fk
			FOREIGN KEY (color_code) REFERENCES colors(code) ON DELETE RESTRICT`},
		{"variants_size_fk", `
			ALTER TABLE product_variants
			ADD CONSTRAINT variants_size_fk
			FOREIGN KEY (size_code) REFERENCES sizes(code) ON DELETE RESTRICT`},
		{"variants_edition_fk", `
			ALTER TABLE product_variants
			ADD CONSTRAINT variants_edition_fk
			FOREIGN KEY (edition_code) REFERENCES sku_editions(code) ON DELETE RESTRICT`},
		{"products_subcategory_fk", `
			ALTER TABLE products
			ADD CONSTRAINT products_subcategory_fk
			FOREIGN KEY (subcategory_code) REFERENCES subcategories(code) ON DELETE RESTRICT`},
		{"products_occasion_fk", `
			ALTER TABLE products
			ADD CONSTRAINT products_occasion_fk
			FOREIGN KEY (occasion_code) REFERENCES occasions(code) ON DELETE RESTRICT`},
		{"products_target_fk", `
			ALTER TABLE products
			ADD CONSTRAINT products_target_fk
			FOREIGN KEY (target_code) REFERENCES targets(code) ON DELETE RESTRICT`},
	}
	for _, fk := range fks {
		var exists bool
		if err := s.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conname = $1)`, fk.name,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check fk %s: %w", fk.name, err)
		}
		if exists {
			continue
		}
		if _, err := s.pool.Exec(ctx, fk.sql); err != nil {
			return fmt.Errorf("add fk %s: %w", fk.name, err)
		}
	}
	return nil
}

func (s *ProductSearchStore) backfillVariantSegmentCodes(ctx context.Context) error {
	rows, err := s.pool.Query(ctx,
		`SELECT sku, color, size, color_code, size_code, edition_code FROM product_variants
		 WHERE color_code IS NULL OR size_code IS NULL`,
	)
	if err != nil {
		return fmt.Errorf("scan variants for segment codes: %w", err)
	}
	defer rows.Close()

	type row struct {
		sku, color, size string
		colorCode, sizeCode, editionCode *string
	}
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.sku, &r.color, &r.size, &r.colorCode, &r.sizeCode, &r.editionCode); err != nil {
			return err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, r := range list {
		colorCode := ""
		if r.colorCode != nil {
			colorCode = *r.colorCode
		}
		sizeCode := ""
		if r.sizeCode != nil {
			sizeCode = *r.sizeCode
		}
		v := domain.Variant{Color: r.color, Size: r.size, ColorCode: colorCode, SizeCode: sizeCode}
		domain.ResolveVariantCodes(&v)
		// Only persist codes that exist in masters (or insert missing from seed resolution).
		if v.ColorCode != "" {
			s.pool.Exec(ctx,
				`INSERT INTO colors (code, name) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING`,
				v.ColorCode, coalesceName(r.color, v.ColorCode),
			)
		}
		if v.SizeCode != "" {
			s.pool.Exec(ctx,
				`INSERT INTO sizes (code, name) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING`,
				v.SizeCode, coalesceName(r.size, v.SizeCode),
			)
		}
		_, err := s.pool.Exec(ctx,
			`UPDATE product_variants
			 SET color_code = COALESCE(color_code, NULLIF($2, '')),
			     size_code  = COALESCE(size_code, NULLIF($3, ''))
			 WHERE sku = $1`,
			r.sku, v.ColorCode, v.SizeCode,
		)
		if err != nil {
			return fmt.Errorf("backfill variant codes for %s: %w", r.sku, err)
		}
	}
	return nil
}

func (s *ProductSearchStore) seedSKUMasters(ctx context.Context) error {
	for _, b := range domain.SeedBrands {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO brands (code, name) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING`,
			b.Code, b.Name,
		); err != nil {
			return fmt.Errorf("seed brand %s: %w", b.Code, err)
		}
	}
	for _, c := range domain.SeedColors {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO colors (code, name) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING`,
			c.Code, c.Name,
		); err != nil {
			return fmt.Errorf("seed color %s: %w", c.Code, err)
		}
	}
	for _, sz := range domain.SeedSizes {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO sizes (code, name) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING`,
			sz.Code, sz.Name,
		); err != nil {
			return fmt.Errorf("seed size %s: %w", sz.Code, err)
		}
	}
	for _, e := range domain.SeedEditions {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO sku_editions (code, name) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING`,
			e.Code, e.Name,
		); err != nil {
			return fmt.Errorf("seed edition %s: %w", e.Code, err)
		}
	}
	for _, sc := range domain.SeedSubcategories {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO subcategories (code, name) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING`,
			sc.Code, sc.Name,
		); err != nil {
			return fmt.Errorf("seed subcategory %s: %w", sc.Code, err)
		}
	}
	for _, o := range domain.SeedOccasions {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO occasions (code, name) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING`,
			o.Code, o.Name,
		); err != nil {
			return fmt.Errorf("seed occasion %s: %w", o.Code, err)
		}
	}
	for _, t := range domain.SeedTargets {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO targets (code, name) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING`,
			t.Code, t.Name,
		); err != nil {
			return fmt.Errorf("seed target %s: %w", t.Code, err)
		}
	}
	return nil
}

func (s *ProductSearchStore) backfillProductSKUCodes(ctx context.Context) error {
	rows, err := s.pool.Query(ctx,
		`SELECT id, brand, COALESCE(brand_code, ''), COALESCE(style_code, '') FROM products
		 WHERE brand_code IS NULL OR style_code IS NULL OR brand_code = '' OR style_code = ''`,
	)
	if err != nil {
		return fmt.Errorf("scan products for sku codes: %w", err)
	}
	defer rows.Close()

	type row struct {
		id, brand, brandCode, styleCode string
	}
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.brand, &r.brandCode, &r.styleCode); err != nil {
			return fmt.Errorf("scan product sku codes: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, r := range list {
		p := domain.Product{ID: r.id, Brand: r.brand, BrandCode: r.brandCode, StyleCode: r.styleCode}
		domain.AssignProductCodes(&p)
		if p.BrandCode == "" || p.StyleCode == "" {
			continue
		}
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO brands (code, name) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING`,
			p.BrandCode, coalesceName(r.brand, p.BrandCode),
		); err != nil {
			return fmt.Errorf("ensure brand %s: %w", p.BrandCode, err)
		}
		if _, err := s.pool.Exec(ctx,
			`UPDATE products SET brand_code = $2, style_code = $3
			 WHERE id = $1 AND (brand_code IS NULL OR style_code IS NULL OR brand_code = '' OR style_code = '')`,
			r.id, p.BrandCode, p.StyleCode,
		); err != nil {
			return fmt.Errorf("backfill product codes for %s: %w", r.id, err)
		}
	}
	return nil
}

func coalesceName(name, fallback string) string {
	if name != "" {
		return name
	}
	return fallback
}

func nullEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// insertStyleForProduct creates a styles row for a new product (idempotent).
// Used on product create so FKs succeed; does not invent brands.
func (s *ProductSearchStore) insertStyleForProduct(ctx context.Context, brandCode, styleCode, name string) error {
	brandCode = domain.NormalizeCode(brandCode)
	styleCode = domain.NormalizeCode(styleCode)
	if brandCode == "" || styleCode == "" {
		return nil
	}
	var brandExists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brands WHERE code = $1)`, brandCode).Scan(&brandExists); err != nil {
		return err
	}
	if !brandExists {
		return fmt.Errorf("%w: brand %s", domain.ErrMasterNotFound, brandCode)
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO styles (brand_code, code, name) VALUES ($1, $2, $3)
		 ON CONFLICT (brand_code, code) DO NOTHING`,
		brandCode, styleCode, coalesceName(name, styleCode),
	)
	return err
}
