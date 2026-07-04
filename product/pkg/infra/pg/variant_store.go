package pg

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgtype"
	"github.com/elug3/dupli1/product/pkg/domain"
)

const variantSelectCols = `sku, product_id, color, size, price, status, image_urls, created_at`

func scanVariant(scan func(...any) error) (domain.Variant, error) {
	var v domain.Variant
	var createdAt time.Time
	var imageURLs pgtype.TextArray
	err := scan(
		&v.SKU, &v.ProductID, &v.Color, &v.Size, &v.Price, &v.Status, &imageURLs, &createdAt,
	)
	if err != nil {
		return domain.Variant{}, err
	}
	v.ImageURLs = scanTextArray(imageURLs)
	v.CreatedAt = createdAt.Format(time.RFC3339)
	return v, nil
}

func (s *ProductSearchStore) ListVariants(productID string) ([]domain.Variant, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT `+variantSelectCols+` FROM product_variants
		 WHERE product_id = $1
		 ORDER BY created_at ASC, sku ASC`,
		productID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.Variant
	for rows.Next() {
		v, err := scanVariant(rows.Scan)
		if err != nil {
			return nil, err
		}
		results = append(results, v)
	}
	return results, rows.Err()
}

func (s *ProductSearchStore) GetVariant(sku string) (*domain.Variant, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT `+variantSelectCols+` FROM product_variants WHERE sku = $1`, sku,
	)
	v, err := scanVariant(row.Scan)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func optionCode(value string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	code := b.String()
	if len(code) > 3 {
		code = code[:3]
	}
	for len(code) < 3 && code != "" {
		code += "X"
	}
	return code
}

func (s *ProductSearchStore) nextVariantSKU(ctx context.Context, productID, color, size string) (string, error) {
	parts := []string{productID}
	if c := optionCode(color); c != "" {
		parts = append(parts, c)
	}
	if sz := optionCode(size); sz != "" {
		parts = append(parts, sz)
	}
	base := strings.Join(parts, "-")
	if base == productID {
		base = productID + "-VAR"
	}

	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM product_variants WHERE sku = $1)`, base).Scan(&exists)
	if err != nil {
		return "", err
	}
	if !exists {
		return base, nil
	}
	for i := 2; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM product_variants WHERE sku = $1)`, candidate).Scan(&exists)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("generate variant sku: exhausted candidates for %s", productID)
}

func (s *ProductSearchStore) CreateVariant(v domain.Variant) (*domain.Variant, error) {
	ctx := context.Background()
	if v.ProductID == "" {
		return nil, fmt.Errorf("productId is required")
	}
	if v.Status == "" {
		v.Status = "active"
	}
	if v.SKU == "" {
		sku, err := s.nextVariantSKU(ctx, v.ProductID, v.Color, v.Size)
		if err != nil {
			return nil, err
		}
		v.SKU = sku
	}

	var createdAt time.Time
	err := s.pool.QueryRow(ctx,
		`INSERT INTO product_variants (sku, product_id, color, size, price, status, image_urls)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING created_at`,
		v.SKU, v.ProductID, v.Color, v.Size, v.Price, v.Status, toTextArray(v.ImageURLs),
	).Scan(&createdAt)
	if err != nil {
		return nil, err
	}
	v.CreatedAt = createdAt.Format(time.RFC3339)
	return &v, nil
}

func (s *ProductSearchStore) UpdateVariant(v domain.Variant) (*domain.Variant, error) {
	var createdAt time.Time
	err := s.pool.QueryRow(context.Background(),
		`UPDATE product_variants
		 SET color=$2, size=$3, price=$4, status=$5, image_urls=$6
		 WHERE sku=$1
		 RETURNING product_id, created_at`,
		v.SKU, v.Color, v.Size, v.Price, v.Status, toTextArray(v.ImageURLs),
	).Scan(&v.ProductID, &createdAt)
	if err != nil {
		return nil, err
	}
	v.CreatedAt = createdAt.Format(time.RFC3339)
	return &v, nil
}

func (s *ProductSearchStore) DeleteVariant(sku string) error {
	cmd, err := s.pool.Exec(context.Background(), `DELETE FROM product_variants WHERE sku = $1`, sku)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("variant not found: %s", sku)
	}
	return nil
}
