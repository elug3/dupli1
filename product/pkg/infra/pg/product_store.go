package pg

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4/pgxpool"
)

type ProductSearchStore struct {
	pool *pgxpool.Pool
}

func NewProductStore(connString string) (*ProductSearchStore, error) {
	connString = withPostgresSSLMode(connString)
	pool, err := pgxpool.Connect(context.Background(), connString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	store := &ProductSearchStore{pool: pool}
	if err := store.migrate(); err != nil {
		store.Close()
		return nil, err
	}
	return store, nil
}

func (s *ProductSearchStore) migrate() error {
	ctx := context.Background()

	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS products (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			price       NUMERIC(10,2) NOT NULL DEFAULT 0,
			brand       TEXT NOT NULL DEFAULT '',
			color       TEXT NOT NULL DEFAULT '',
			material    TEXT NOT NULL DEFAULT '',
			stock       INTEGER NOT NULL DEFAULT 0,
			category    TEXT NOT NULL DEFAULT '',
			status      TEXT NOT NULL DEFAULT 'active',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("migrate products: %w", err)
	}

	for _, col := range []struct{ name, def string }{
		{"category", "TEXT NOT NULL DEFAULT ''"},
		{"status", "TEXT NOT NULL DEFAULT 'active'"},
		{"created_at", "TIMESTAMPTZ NOT NULL DEFAULT NOW()"},
		{"image_url", "TEXT NOT NULL DEFAULT ''"},
		{"image_urls", "TEXT[] NOT NULL DEFAULT '{}'"},
		{"capacity", "TEXT NOT NULL DEFAULT ''"},
		{"tags", "TEXT[] NOT NULL DEFAULT '{}'"},
		{"created_by", "TEXT NOT NULL DEFAULT ''"},
	} {
		s.pool.Exec(ctx, fmt.Sprintf(
			"ALTER TABLE products ADD COLUMN IF NOT EXISTS %s %s", col.name, col.def,
		))
	}

	s.pool.Exec(ctx,
		`UPDATE products SET image_urls = ARRAY[image_url] WHERE image_url != '' AND image_urls = '{}'`,
	)

	_, err = s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS product_variants (
			sku            TEXT PRIMARY KEY,
			product_id     TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
			color          TEXT NOT NULL DEFAULT '',
			size           TEXT NOT NULL DEFAULT '',
			selling_price  NUMERIC(10,2) NOT NULL DEFAULT 0,
			price          NUMERIC(10,2) NOT NULL DEFAULT 0,
			status         TEXT NOT NULL DEFAULT 'active',
			image_urls     TEXT[] NOT NULL DEFAULT '{}',
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (product_id, color, size)
		)
	`)
	if err != nil {
		return fmt.Errorf("migrate product_variants: %w", err)
	}

	s.pool.Exec(ctx,
		`ALTER TABLE product_variants ADD COLUMN IF NOT EXISTS selling_price NUMERIC(10,2) NOT NULL DEFAULT 0`,
	)
	s.pool.Exec(ctx, `ALTER TABLE product_variants ADD COLUMN IF NOT EXISTS sku_id TEXT`)

	s.pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_product_variants_product_id ON product_variants(product_id)`)
	s.pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_products_status_created_at ON products(status, created_at DESC)`)
	s.pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_products_category ON products(category)`)
	s.pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_products_tags ON products USING GIN (tags)`)
	s.pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_product_variants_product_status_color ON product_variants(product_id, status, color)`)
	s.pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_product_variants_product_status_size ON product_variants(product_id, status, size)`)

	if err := s.backfillVariants(ctx); err != nil {
		return err
	}
	if err := s.backfillSkuIDs(ctx); err != nil {
		return err
	}
	if err := s.promoteSkuIDPrimaryKey(ctx); err != nil {
		return err
	}
	if err := s.migrateSKUMasters(ctx); err != nil {
		return err
	}
	return nil
}

// promoteSkuIDPrimaryKey makes sku_id the real primary key of
// product_variants (converting the existing unique index into the PK
// constraint, so no index rebuild is needed) and demotes sku to a plain
// unique, still-NOT-NULL column. Safe to run on every startup: it checks the
// current primary key column first and does nothing once already promoted.
// By this point sku_id is always NOT NULL (backfillSkuIDs guarantees it), so
// there's no gating needed here, unlike order's cross-service backfill.
func (s *ProductSearchStore) promoteSkuIDPrimaryKey(ctx context.Context) error {
	var pkColumn string
	err := s.pool.QueryRow(ctx, `
		SELECT a.attname
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = 'product_variants'::regclass AND i.indisprimary
		LIMIT 1
	`).Scan(&pkColumn)
	if err != nil {
		return fmt.Errorf("check product_variants primary key: %w", err)
	}
	if pkColumn == "sku_id" {
		return nil
	}

	if _, err := s.pool.Exec(ctx, `ALTER TABLE product_variants DROP CONSTRAINT product_variants_pkey`); err != nil {
		return fmt.Errorf("drop legacy product_variants pkey: %w", err)
	}
	if _, err := s.pool.Exec(ctx,
		`ALTER TABLE product_variants ADD CONSTRAINT product_variants_pkey PRIMARY KEY USING INDEX ux_product_variants_sku_id`,
	); err != nil {
		return fmt.Errorf("promote sku_id to primary key: %w", err)
	}
	if _, err := s.pool.Exec(ctx,
		`ALTER TABLE product_variants ADD CONSTRAINT product_variants_sku_key UNIQUE (sku)`,
	); err != nil {
		return fmt.Errorf("add sku unique constraint: %w", err)
	}
	return nil
}

// backfillSkuIDs assigns a canonical ULID sku_id to every variant row that
// doesn't have one yet, then locks the column down (NOT NULL + unique index).
// Runs to completion before the server accepts traffic, so CreateVariant only
// ever needs to assign a sku_id for the single new row it's inserting.
func (s *ProductSearchStore) backfillSkuIDs(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `SELECT sku FROM product_variants WHERE sku_id IS NULL OR sku_id = ''`)
	if err != nil {
		return fmt.Errorf("scan variants missing sku_id: %w", err)
	}
	var skus []string
	for rows.Next() {
		var sku string
		if err := rows.Scan(&sku); err != nil {
			rows.Close()
			return fmt.Errorf("scan variants missing sku_id: %w", err)
		}
		skus = append(skus, sku)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("scan variants missing sku_id: %w", err)
	}

	for _, sku := range skus {
		if _, err := s.pool.Exec(ctx,
			`UPDATE product_variants SET sku_id = $2 WHERE sku = $1 AND (sku_id IS NULL OR sku_id = '')`,
			sku, domain.NewSkuID(),
		); err != nil {
			return fmt.Errorf("backfill sku_id for %s: %w", sku, err)
		}
	}

	if _, err := s.pool.Exec(ctx, `ALTER TABLE product_variants ALTER COLUMN sku_id SET NOT NULL`); err != nil {
		return fmt.Errorf("set sku_id not null: %w", err)
	}
	if _, err := s.pool.Exec(ctx,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_product_variants_sku_id ON product_variants(sku_id)`,
	); err != nil {
		return fmt.Errorf("create sku_id unique index: %w", err)
	}
	return nil
}

// backfillVariants creates one variant per legacy product row that has none yet.
// SKU equals the product id so existing inventory/order references keep working.
func (s *ProductSearchStore) backfillVariants(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO product_variants (sku, product_id, color, size, selling_price, price, status, image_urls, created_at)
		SELECT p.id, p.id, p.color, '', p.price, p.price, p.status, p.image_urls, p.created_at
		FROM products p
		WHERE NOT EXISTS (
			SELECT 1 FROM product_variants v WHERE v.product_id = p.id
		)
		ON CONFLICT (sku) DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("backfill product_variants: %w", err)
	}
	return nil
}

func (s *ProductSearchStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *ProductSearchStore) Pool() *pgxpool.Pool {
	return s.pool
}

func brandPrefix(brand string) string {
	if code := domain.BrandCodeFromName(brand); code != "" {
		return code
	}
	return "PRD"
}

func (s *ProductSearchStore) nextProductID(ctx context.Context, brand string) (string, error) {
	prefix := brandPrefix(brand)
	var seq int
	// $1 must be cast to int: SUBSTRING(text FROM ...) is ambiguous between the
	// positional form (int) and the regex form (text). Without the cast Postgres
	// infers $1 as text, so the driver rejects the integer offset with
	// "cannot convert N to Text" and product-ID generation fails.
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(CAST(SUBSTRING(id FROM $1::int) AS INTEGER)), 0)
		 FROM products WHERE id ~ ('^' || $2 || '-[0-9]+$')`,
		len(prefix)+2, prefix,
	).Scan(&seq)
	if err != nil {
		return "", fmt.Errorf("generate product id: %w", err)
	}
	return fmt.Sprintf("%s-%03d", prefix, seq+1), nil
}

func scanTextArray(arr pgtype.TextArray) []string {
	if arr.Status != pgtype.Present {
		return nil
	}
	out := make([]string, 0, len(arr.Elements))
	for _, e := range arr.Elements {
		if e.Status == pgtype.Present && e.String != "" {
			out = append(out, e.String)
		}
	}
	return out
}

func toTextArray(ss []string) pgtype.TextArray {
	if len(ss) == 0 {
		return pgtype.TextArray{Status: pgtype.Present}
	}
	elems := make([]pgtype.Text, len(ss))
	for i, s := range ss {
		elems[i] = pgtype.Text{String: s, Status: pgtype.Present}
	}
	return pgtype.TextArray{
		Elements:   elems,
		Dimensions: []pgtype.ArrayDimension{{Length: int32(len(ss)), LowerBound: 1}},
		Status:     pgtype.Present,
	}
}

const parentSelectCols = `id, name, description, brand, brand_code, style_code, material, category, status, capacity, tags, created_at, created_by`

func scanParent(scan func(...any) error) (domain.Product, error) {
	var p domain.Product
	var createdAt time.Time
	var tags pgtype.TextArray
	var capacity string
	var brandCode, styleCode *string
	err := scan(
		&p.ID, &p.Name, &p.Description,
		&p.Brand, &brandCode, &styleCode, &p.Material, &p.Category, &p.Status,
		&capacity, &tags, &createdAt, &p.CreatedBy,
	)
	if err != nil {
		return domain.Product{}, err
	}
	if brandCode != nil {
		p.BrandCode = *brandCode
	}
	if styleCode != nil {
		p.StyleCode = *styleCode
	}
	p.Capacity = capacity
	p.Tags = scanTextArray(tags)
	p.CreatedAt = createdAt.Format(time.RFC3339)
	return p, nil
}

func (s *ProductSearchStore) enrich(products []domain.Product, includeVariants bool) error {
	if len(products) == 0 {
		return nil
	}
	ids := make([]string, len(products))
	for i, p := range products {
		ids[i] = p.ID
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT `+variantSelectCols+`
		 FROM product_variants
		 WHERE product_id = ANY($1)
		 ORDER BY created_at ASC, sku ASC`,
		toTextArray(ids),
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	byProduct := make(map[string][]domain.Variant, len(products))
	for rows.Next() {
		v, err := scanVariant(rows.Scan)
		if err != nil {
			return err
		}
		byProduct[v.ProductID] = append(byProduct[v.ProductID], v)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for i := range products {
		products[i].EnrichFromVariants(byProduct[products[i].ID], includeVariants)
	}
	s.enrichMasterNames(products)
	return nil
}

func (s *ProductSearchStore) SearchProducts(filter map[string]string) ([]domain.Product, int, error) {
	where, args := buildProductSearchWhere(filter)
	countQuery := "SELECT COUNT(*) FROM products p WHERE 1=1" + where
	var total int
	if err := s.pool.QueryRow(context.Background(), countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := "SELECT " + parentSelectCols + " FROM products p WHERE 1=1" + where
	query += " ORDER BY p.created_at DESC"

	limit, hasLimit := atoiFilter(filter, "limit")
	offset, _ := atoiFilter(filter, "offset")
	if hasLimit && limit > 0 {
		if offset < 0 {
			offset = 0
		}
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
		args = append(args, limit, offset)
	}

	rows, err := s.pool.Query(context.Background(), query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []domain.Product
	for rows.Next() {
		p, err := scanParent(rows.Scan)
		if err != nil {
			return nil, 0, err
		}
		results = append(results, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if err := s.enrich(results, false); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

func buildProductSearchWhere(filter map[string]string) (string, []interface{}) {
	query := ""
	args := []interface{}{}
	idx := 1

	for key, value := range filter {
		switch key {
		case "category":
			query += fmt.Sprintf(" AND p.category = $%d", idx)
			args = append(args, value)
			idx++
		case "brand":
			query += fmt.Sprintf(" AND p.brand ILIKE $%d", idx)
			args = append(args, "%"+value+"%")
			idx++
		case "material":
			query += fmt.Sprintf(" AND p.material = $%d", idx)
			args = append(args, value)
			idx++
		case "status":
			query += fmt.Sprintf(" AND p.status = $%d", idx)
			args = append(args, value)
			idx++
		case "tags":
			tagList := splitTags(value)
			if len(tagList) == 0 {
				continue
			}
			query += fmt.Sprintf(" AND p.tags @> $%d::text[]", idx)
			args = append(args, tagList)
			idx++
		case "color":
			query += fmt.Sprintf(` AND EXISTS (
				SELECT 1 FROM product_variants v
				WHERE v.product_id = p.id AND v.color = $%d AND v.status = 'active'
			)`, idx)
			args = append(args, value)
			idx++
		case "size":
			query += fmt.Sprintf(` AND EXISTS (
				SELECT 1 FROM product_variants v
				WHERE v.product_id = p.id AND v.size = $%d AND v.status = 'active'
			)`, idx)
			args = append(args, value)
			idx++
		}
	}
	return query, args
}

func atoiFilter(filter map[string]string, key string) (int, bool) {
	raw, ok := filter[key]
	if !ok || raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return n, true
}

func splitTags(value string) []string {
	parts := strings.Split(value, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			tags = append(tags, part)
		}
	}
	return tags
}

func (s *ProductSearchStore) ListProducts() ([]domain.Product, error) {
	results, _, err := s.SearchProducts(nil)
	return results, err
}

func (s *ProductSearchStore) GetActiveProduct(id string) (*domain.Product, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT `+parentSelectCols+` FROM products WHERE id = $1 AND status = 'active'`, id,
	)
	p, err := scanParent(row.Scan)
	if err != nil {
		return nil, err
	}
	variants, err := s.ListVariants(id)
	if err != nil {
		return nil, err
	}
	active := make([]domain.Variant, 0, len(variants))
	for _, v := range variants {
		if v.Status == "active" {
			active = append(active, v)
		}
	}
	p.EnrichFromVariants(active, true)
	products := []domain.Product{p}
	s.enrichMasterNames(products)
	return &products[0], nil
}

func (s *ProductSearchStore) GetProduct(id string) (*domain.Product, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT `+parentSelectCols+` FROM products WHERE id = $1`, id,
	)
	p, err := scanParent(row.Scan)
	if err != nil {
		return nil, err
	}
	variants, err := s.ListVariants(id)
	if err != nil {
		return nil, err
	}
	p.EnrichFromVariants(variants, true)
	products := []domain.Product{p}
	s.enrichMasterNames(products)
	return &products[0], nil
}

func (s *ProductSearchStore) CreateProduct(p domain.Product) (*domain.Product, error) {
	ctx := context.Background()
	if p.ID == "" {
		p.ID = domain.NewProductID()
	}
	if p.Status == "" {
		p.Status = "active"
	}
	if err := domain.RequireProductSKUCodes(&p); err != nil {
		return nil, err
	}
	if err := s.requireBrand(ctx, p.BrandCode); err != nil {
		return nil, err
	}
	if err := s.requireStyle(ctx, p.BrandCode, p.StyleCode); err != nil {
		return nil, err
	}
	if p.Brand == "" {
		p.Brand = s.brandName(ctx, p.BrandCode)
	}

	var createdAt time.Time
	err := s.pool.QueryRow(ctx,
		`INSERT INTO products (id, name, description, price, brand, brand_code, style_code, color, material, stock, category, status, image_urls, capacity, tags, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		 RETURNING created_at`,
		p.ID, p.Name, p.Description, p.Price,
		p.Brand, nullEmpty(p.BrandCode), nullEmpty(p.StyleCode), p.Color, p.Material, p.Stock, p.Category, p.Status,
		toTextArray(p.ImageURLs), p.Capacity, toTextArray(p.Tags), p.CreatedBy,
	).Scan(&createdAt)
	if err != nil {
		return nil, err
	}
	p.CreatedAt = createdAt.Format(time.RFC3339)

	switch {
	case len(p.Variants) > 0:
		for _, v := range p.Variants {
			v.ProductID = p.ID
			if v.Status == "" {
				v.Status = p.Status
			}
			if _, err := s.CreateVariant(v); err != nil {
				return nil, err
			}
		}
	case p.Color != "" || p.Price > 0 || p.SellingPrice > 0 || len(p.ImageURLs) > 0:
		// Legacy create: seed a default variant; SKU is composed from masters (not product id).
		if _, err := s.CreateVariant(domain.Variant{
			ProductID:    p.ID,
			Color:        p.Color,
			SellingPrice: p.SellingPrice,
			Price:        p.Price,
			Status:       p.Status,
			ImageURLs:    p.ImageURLs,
		}); err != nil {
			return nil, err
		}
	}

	return s.GetProduct(p.ID)
}

func (s *ProductSearchStore) UpdateProduct(p domain.Product) (*domain.Product, error) {
	var createdAt time.Time
	err := s.pool.QueryRow(context.Background(),
		`UPDATE products
		 SET name=$2, description=$3, brand=$4, material=$5, category=$6, status=$7, capacity=$8, tags=$9
		 WHERE id=$1
		 RETURNING created_at`,
		p.ID, p.Name, p.Description,
		p.Brand, p.Material, p.Category, p.Status,
		p.Capacity, toTextArray(p.Tags),
	).Scan(&createdAt)
	if err != nil {
		return nil, err
	}
	return s.GetProduct(p.ID)
}

func (s *ProductSearchStore) DeleteProduct(id string) error {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("delete product: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// products -> product_variants is ON DELETE CASCADE, but stock_items has an
	// ON DELETE RESTRICT FK to product_variants(sku_id). So deleting a product
	// that has any stock rows is blocked by stock_items_sku_id_fkey. Clear the
	// product's stock rows first, then the cascade can remove the variants.
	// (reservation_items intentionally has no FK to variants, so it isn't a blocker.)
	if _, err := tx.Exec(ctx,
		`DELETE FROM stock_items
		 WHERE sku_id IN (SELECT sku_id FROM product_variants WHERE product_id = $1)`,
		id,
	); err != nil {
		return fmt.Errorf("delete product stock: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM products WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete product: %w", err)
	}

	return tx.Commit(ctx)
}
