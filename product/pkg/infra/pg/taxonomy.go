package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/jackc/pgx/v4"
)

// taxonomyTables maps each merchandising kind to its table and to the
// products column that carries its code.
var taxonomyTables = map[domain.TaxonomyKind]struct{ table, productColumn string }{
	domain.TaxonomySubCategory: {"bag_subcategories", "sub_category"},
	domain.TaxonomyBagStyle:    {"bag_styles", "bag_style"},
	domain.TaxonomyTarget:      {"bag_targets", "target"},
}

// legacyClothingSubCategories are the clothing subcategories the service
// served from code before staff could edit the taxonomy. The bag terms were
// already rows (the old migrate wrote them on every start); these were not,
// because the table had no category column. They are copied once, by the
// migrate that adds that column, so clothing products stay editable on an
// environment upgraded in place. A fresh database gets nothing here: its
// terms come from scripts/seed-reference-data.py.
var legacyClothingSubCategories = []domain.CatalogTerm{
	{Code: "padded", Name: "Padded Jackets", Category: "clothing", SortOrder: 0},
	{Code: "jackets", Name: "Jackets", Category: "clothing", SortOrder: 1},
}

// migrateProductTaxonomy adds the merchandising columns on products and the
// staff-edited term tables for subcategory / bag style / target (storefront
// filters; not SKU segments). It seeds no terms.
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

	// Before the category column existed, every subcategory row was a bag.
	var upgrading bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.tables
		               WHERE table_schema = current_schema() AND table_name = 'bag_subcategories')
		   AND NOT EXISTS (SELECT 1 FROM information_schema.columns
		               WHERE table_schema = current_schema() AND table_name = 'bag_subcategories'
		                 AND column_name = 'category')`,
	).Scan(&upgrading); err != nil {
		return fmt.Errorf("migrate product taxonomy: %w", err)
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS bag_subcategories (
			code TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL DEFAULT 'bags',
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
		`ALTER TABLE bag_subcategories ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'bags'`,
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

	if upgrading {
		for _, t := range legacyClothingSubCategories {
			if _, err := s.pool.Exec(ctx,
				`INSERT INTO bag_subcategories (code, name, category, sort_order) VALUES ($1, $2, $3, $4)
				 ON CONFLICT (code) DO NOTHING`,
				t.Code, t.Name, t.Category, t.SortOrder,
			); err != nil {
				return fmt.Errorf("migrate clothing subcategory %s: %w", t.Code, err)
			}
		}
	}
	return nil
}

func taxonomyTable(kind domain.TaxonomyKind) (string, string, error) {
	t, ok := taxonomyTables[kind]
	if !ok {
		return "", "", fmt.Errorf("unknown taxonomy kind %q", kind)
	}
	return t.table, t.productColumn, nil
}

// termColumns selects a term; only subcategories have a category.
func termColumns(kind domain.TaxonomyKind) string {
	if kind == domain.TaxonomySubCategory {
		return "code, name, category, sort_order"
	}
	return "code, name, '' AS category, sort_order"
}

func (s *CatalogStore) ListTaxonomy(ctx context.Context, kind domain.TaxonomyKind) ([]domain.CatalogTerm, error) {
	table, _, err := taxonomyTable(kind)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM %s ORDER BY sort_order ASC, code ASC`, termColumns(kind), table))
	if err != nil {
		return nil, wrapDB("list "+table, err)
	}
	defer rows.Close()
	out := []domain.CatalogTerm{}
	for rows.Next() {
		var t domain.CatalogTerm
		if err := rows.Scan(&t.Code, &t.Name, &t.Category, &t.SortOrder); err != nil {
			return nil, wrapDB("list "+table, err)
		}
		out = append(out, t)
	}
	return out, wrapDB("list "+table, rows.Err())
}

func (s *CatalogStore) CreateTaxonomyTerm(ctx context.Context, kind domain.TaxonomyKind, t domain.CatalogTerm) (*domain.CatalogTerm, error) {
	table, _, err := taxonomyTable(kind)
	if err != nil {
		return nil, err
	}
	if kind == domain.TaxonomySubCategory {
		_, err = s.pool.Exec(ctx,
			`INSERT INTO bag_subcategories (code, name, category, sort_order) VALUES ($1, $2, $3, $4)`,
			t.Code, t.Name, t.Category, t.SortOrder)
	} else {
		t.Category = ""
		_, err = s.pool.Exec(ctx,
			fmt.Sprintf(`INSERT INTO %s (code, name, sort_order) VALUES ($1, $2, $3)`, table),
			t.Code, t.Name, t.SortOrder)
	}
	if isUniqueViolation(err) {
		return nil, domain.ErrMasterExists
	}
	if err != nil {
		return nil, wrapDB("create "+table, err)
	}
	return &t, nil
}

func (s *CatalogStore) UpdateTaxonomyTerm(ctx context.Context, kind domain.TaxonomyKind, code string, patch domain.CatalogTermPatch) (*domain.CatalogTerm, error) {
	table, _, err := taxonomyTable(kind)
	if err != nil {
		return nil, err
	}
	var t domain.CatalogTerm
	err = s.pool.QueryRow(ctx, fmt.Sprintf(
		`UPDATE %s SET name = COALESCE($2, name), sort_order = COALESCE($3, sort_order)
		 WHERE code = $1 RETURNING %s`, table, termColumns(kind)),
		code, patch.Name, patch.SortOrder,
	).Scan(&t.Code, &t.Name, &t.Category, &t.SortOrder)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrMasterNotFound
	}
	if err != nil {
		return nil, wrapDB("update "+table, err)
	}
	return &t, nil
}

func (s *CatalogStore) DeleteTaxonomyTerm(ctx context.Context, kind domain.TaxonomyKind, code string) error {
	table, column, err := taxonomyTable(kind)
	if err != nil {
		return err
	}
	// One statement, so a product written between a check and the delete
	// cannot slip through: the row goes only while no product carries it.
	var deleted, exists bool
	err = s.pool.QueryRow(ctx, fmt.Sprintf(`
		WITH gone AS (
			DELETE FROM %[1]s WHERE code = $1
			  AND NOT EXISTS (SELECT 1 FROM products WHERE %[2]s = $1)
			RETURNING code
		)
		SELECT EXISTS (SELECT 1 FROM gone), EXISTS (SELECT 1 FROM %[1]s WHERE code = $1)`, table, column),
		code,
	).Scan(&deleted, &exists)
	if err != nil {
		return wrapDB("delete "+table, err)
	}
	switch {
	case deleted:
		return nil
	case exists:
		return domain.ErrMasterInUse
	default:
		return domain.ErrMasterNotFound
	}
}
