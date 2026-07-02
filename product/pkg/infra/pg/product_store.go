package pg

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/elug3/dupli1/product/pkg/domain"
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

	// Add columns that may be missing from an older schema.
	for _, col := range []struct{ name, def string }{
		{"category", "TEXT NOT NULL DEFAULT ''"},
		{"status", "TEXT NOT NULL DEFAULT 'active'"},
		{"created_at", "TIMESTAMPTZ NOT NULL DEFAULT NOW()"},
		{"image_url", "TEXT NOT NULL DEFAULT ''"},
		{"cost", "NUMERIC(10,2) NOT NULL DEFAULT 0"},
		{"image_urls", "TEXT[] NOT NULL DEFAULT '{}'"},
		{"capacity", "TEXT NOT NULL DEFAULT ''"},
	} {
		s.pool.Exec(ctx, fmt.Sprintf(
			"ALTER TABLE products ADD COLUMN IF NOT EXISTS %s %s", col.name, col.def,
		))
	}

	// Migrate existing single image_url values into image_urls array.
	s.pool.Exec(ctx,
		`UPDATE products SET image_urls = ARRAY[image_url] WHERE image_url != '' AND image_urls = '{}'`,
	)

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

// ── helpers ───────────────────────────────────────────────────────────────────

func brandPrefix(brand string) string {
	fields := strings.Fields(strings.TrimSpace(brand))
	word := "PRD"
	if len(fields) > 0 {
		word = fields[0]
	}
	runes := []rune(strings.ToUpper(word))
	if len(runes) > 3 {
		runes = runes[:3]
	}
	for len(runes) < 3 {
		runes = append(runes, 'X')
	}
	return string(runes)
}

func (s *ProductSearchStore) nextProductID(ctx context.Context, brand string) (string, error) {
	prefix := brandPrefix(brand)
	var seq int
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(CAST(SUBSTRING(id FROM $1) AS INTEGER)), 0)
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

// ── queries ───────────────────────────────────────────────────────────────────

const selectCols = `id, name, description, price, cost, brand, color, material, stock, category, status, image_urls, capacity, created_at`

func scanProduct(scan func(...any) error) (domain.Product, error) {
	var p domain.Product
	var createdAt time.Time
	var imageURLs pgtype.TextArray
	var capacity string
	err := scan(
		&p.ID, &p.Name, &p.Description, &p.Price, &p.Cost,
		&p.Brand, &p.Color, &p.Material, &p.Stock,
		&p.Category, &p.Status, &imageURLs, &capacity, &createdAt,
	)
	if err != nil {
		return domain.Product{}, err
	}
	p.ImageURLs = scanTextArray(imageURLs)
	p.Capacity = capacity
	p.CreatedAt = createdAt.Format(time.RFC3339)
	return p, nil
}

func scanBag(scan func(...any) error) (domain.Bag, error) {
	p, err := scanProduct(scan)
	if err != nil {
		return domain.Bag{}, err
	}
	return domain.Bag{Product: p}, nil
}

func (s *ProductSearchStore) SearchBags(filter map[string]string) ([]domain.Bag, error) {
	query := "SELECT " + selectCols + " FROM products WHERE category = 'bags' AND status = 'active'"
	args := []interface{}{}
	idx := 1

	for key, value := range filter {
		switch key {
		case "brand":
			query += fmt.Sprintf(" AND brand ILIKE $%d", idx)
			args = append(args, "%"+value+"%")
			idx++
		case "color":
			query += fmt.Sprintf(" AND color = $%d", idx)
			args = append(args, value)
			idx++
		case "material":
			query += fmt.Sprintf(" AND material = $%d", idx)
			args = append(args, value)
			idx++
		}
	}

	rows, err := s.pool.Query(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.Bag
	for rows.Next() {
		b, err := scanBag(rows.Scan)
		if err != nil {
			return nil, err
		}
		b.Product.Cost = 0
		results = append(results, b)
	}

	return results, rows.Err()
}

func (s *ProductSearchStore) ListProducts() ([]domain.Product, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT `+selectCols+` FROM products ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.Product
	for rows.Next() {
		p, err := scanProduct(rows.Scan)
		if err != nil {
			return nil, err
		}
		results = append(results, p)
	}

	return results, rows.Err()
}

func (s *ProductSearchStore) GetActiveProduct(id string) (*domain.Product, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT `+selectCols+` FROM products WHERE id = $1 AND status = 'active'`, id,
	)
	p, err := scanProduct(row.Scan)
	if err != nil {
		return nil, err
	}
	p.Cost = 0
	return &p, nil
}

func (s *ProductSearchStore) GetProduct(id string) (*domain.Product, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT `+selectCols+` FROM products WHERE id = $1`, id,
	)
	p, err := scanProduct(row.Scan)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *ProductSearchStore) CreateProduct(p domain.Product) (*domain.Product, error) {
	if p.ID == "" {
		id, err := s.nextProductID(context.Background(), p.Brand)
		if err != nil {
			return nil, err
		}
		p.ID = id
	}
	if p.Status == "" {
		p.Status = "active"
	}

	var createdAt time.Time
	err := s.pool.QueryRow(context.Background(),
		`INSERT INTO products (id, name, description, price, cost, brand, color, material, stock, category, status, image_urls, capacity)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 RETURNING created_at`,
		p.ID, p.Name, p.Description, p.Price, p.Cost,
		p.Brand, p.Color, p.Material, p.Stock, p.Category, p.Status,
		toTextArray(p.ImageURLs), p.Capacity,
	).Scan(&createdAt)
	if err != nil {
		return nil, err
	}
	p.CreatedAt = createdAt.Format(time.RFC3339)
	return &p, nil
}

func (s *ProductSearchStore) UpdateProduct(p domain.Product) (*domain.Product, error) {
	var createdAt time.Time
	err := s.pool.QueryRow(context.Background(),
		`UPDATE products
		 SET name=$2, description=$3, price=$4, cost=$5, brand=$6, color=$7, material=$8, stock=$9, category=$10, status=$11, image_urls=$12, capacity=$13
		 WHERE id=$1
		 RETURNING created_at`,
		p.ID, p.Name, p.Description, p.Price, p.Cost,
		p.Brand, p.Color, p.Material, p.Stock, p.Category, p.Status,
		toTextArray(p.ImageURLs), p.Capacity,
	).Scan(&createdAt)
	if err != nil {
		return nil, err
	}
	p.CreatedAt = createdAt.Format(time.RFC3339)
	return &p, nil
}

func (s *ProductSearchStore) DeleteProduct(id string) error {
	_, err := s.pool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id)
	return err
}
