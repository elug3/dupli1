package pg

import (
	"context"
	"fmt"

	"github.com/elug3/dupli1/product/pkg/domain"
)

// migrateSKUMasters creates brand/color/size/edition master tables, adds
// brand_code/style_code on products and segment codes on product_variants,
// seeds well-known rows, and backfills codes on existing products.
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
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS brand_code TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS style_code TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE product_variants ADD COLUMN IF NOT EXISTS color_code TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE product_variants ADD COLUMN IF NOT EXISTS edition_code TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE product_variants ADD COLUMN IF NOT EXISTS size_code TEXT NOT NULL DEFAULT ''`,
	}
	for _, q := range statements {
		if _, err := s.pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migrate sku masters: %w", err)
		}
	}

	if err := s.seedSKUMasters(ctx); err != nil {
		return err
	}
	if err := s.backfillProductSKUCodes(ctx); err != nil {
		return err
	}
	// Unique style within brand (ignore empty legacy rows still being filled).
	if _, err := s.pool.Exec(ctx, `
		CREATE UNIQUE INDEX IF NOT EXISTS ux_products_brand_style
		ON products (brand_code, style_code)
		WHERE brand_code <> '' AND style_code <> ''
	`); err != nil {
		return fmt.Errorf("create brand/style unique index: %w", err)
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
	return nil
}

func (s *ProductSearchStore) backfillProductSKUCodes(ctx context.Context) error {
	rows, err := s.pool.Query(ctx,
		`SELECT id, brand, brand_code, style_code FROM products
		 WHERE brand_code = '' OR style_code = ''`,
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
		// Ensure brand master row exists for backfilled codes.
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO brands (code, name) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING`,
			p.BrandCode, coalesceName(r.brand, p.BrandCode),
		); err != nil {
			return fmt.Errorf("ensure brand %s: %w", p.BrandCode, err)
		}
		if _, err := s.pool.Exec(ctx,
			`UPDATE products SET brand_code = $2, style_code = $3
			 WHERE id = $1 AND (brand_code = '' OR style_code = '')`,
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

// ensureBrand inserts the brand master row if missing (idempotent).
func (s *ProductSearchStore) ensureBrand(ctx context.Context, code, name string) error {
	code = domain.NormalizeCode(code)
	if !domain.ValidBrandCode(code) {
		return fmt.Errorf("invalid brand code %q", code)
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO brands (code, name) VALUES ($1, $2)
		 ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name
		 WHERE brands.name = '' OR brands.name = brands.code`,
		code, coalesceName(name, code),
	)
	return err
}

// ensureColor inserts a color master row if missing.
func (s *ProductSearchStore) ensureColor(ctx context.Context, code, name string) error {
	code = domain.NormalizeCode(code)
	if !domain.ValidSegmentCode(code) {
		return fmt.Errorf("invalid color code %q", code)
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO colors (code, name) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING`,
		code, coalesceName(name, code),
	)
	return err
}

// ensureSize inserts a size master row if missing.
func (s *ProductSearchStore) ensureSize(ctx context.Context, code, name string) error {
	code = domain.NormalizeCode(code)
	if !domain.ValidSegmentCode(code) {
		return fmt.Errorf("invalid size code %q", code)
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sizes (code, name) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING`,
		code, coalesceName(name, code),
	)
	return err
}

// ensureEdition inserts an edition master row if missing.
func (s *ProductSearchStore) ensureEdition(ctx context.Context, code, name string) error {
	code = domain.NormalizeCode(code)
	if code == "" {
		return nil
	}
	if !domain.ValidSegmentCode(code) {
		return fmt.Errorf("invalid edition code %q", code)
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sku_editions (code, name) VALUES ($1, $2) ON CONFLICT (code) DO NOTHING`,
		code, coalesceName(name, code),
	)
	return err
}
