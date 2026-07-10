package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgtype"
)

const variantSelectCols = `sku_id, sku, product_id, color, size, selling_price, price, status, image_urls, created_at`

func scanVariant(scan func(...any) error) (domain.Variant, error) {
	var v domain.Variant
	var createdAt time.Time
	var imageURLs pgtype.TextArray
	err := scan(
		&v.SkuID, &v.SKU, &v.ProductID, &v.Color, &v.Size, &v.SellingPrice, &v.Price, &v.Status, &imageURLs, &createdAt,
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

func (s *ProductSearchStore) GetVariantBySkuID(skuID string) (*domain.Variant, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT `+variantSelectCols+` FROM product_variants WHERE sku_id = $1`, skuID,
	)
	v, err := scanVariant(row.Scan)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *ProductSearchStore) nextVariantSKU(ctx context.Context, productID, color, size string) (string, error) {
	base := domain.BuildVariantSKUBase(productID, color, size)

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
	if v.SkuID == "" {
		v.SkuID = domain.NewSkuID()
	}

	var createdAt time.Time
	err := s.pool.QueryRow(ctx,
		`INSERT INTO product_variants (sku_id, sku, product_id, color, size, selling_price, price, status, image_urls)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING created_at`,
		v.SkuID, v.SKU, v.ProductID, v.Color, v.Size, v.SellingPrice, v.Price, v.Status, toTextArray(v.ImageURLs),
	).Scan(&createdAt)
	if err != nil {
		return nil, err
	}
	v.CreatedAt = createdAt.Format(time.RFC3339)
	return &v, nil
}

// UpdateVariant updates a variant by its (immutable) sku. sku_id is never
// written here — it's read back via RETURNING so the response is always
// correct regardless of what the caller's v.SkuID happened to be set to.
func (s *ProductSearchStore) UpdateVariant(v domain.Variant) (*domain.Variant, error) {
	var createdAt time.Time
	err := s.pool.QueryRow(context.Background(),
		`UPDATE product_variants
		 SET color=$2, size=$3, selling_price=$4, price=$5, status=$6, image_urls=$7
		 WHERE sku=$1
		 RETURNING sku_id, product_id, created_at`,
		v.SKU, v.Color, v.Size, v.SellingPrice, v.Price, v.Status, toTextArray(v.ImageURLs),
	).Scan(&v.SkuID, &v.ProductID, &createdAt)
	if err != nil {
		return nil, err
	}
	v.CreatedAt = createdAt.Format(time.RFC3339)
	return &v, nil
}

func (s *ProductSearchStore) DeleteVariant(sku string) error {
	cmd, err := s.pool.Exec(context.Background(), `DELETE FROM product_variants WHERE sku = $1`, sku)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return fmt.Errorf("cannot delete variant %s: stock exists for it in inventory", sku)
		}
		return err
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("variant not found: %s", sku)
	}
	return nil
}
