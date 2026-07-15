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

const variantSelectCols = `sku_id, sku, product_id, color, size,
	COALESCE(color_code, ''), COALESCE(edition_code, ''), COALESCE(size_code, ''),
	selling_price, price, status, image_urls, created_at`

func scanVariant(scan func(...any) error) (domain.Variant, error) {
	var v domain.Variant
	var createdAt time.Time
	var imageURLs pgtype.TextArray
	err := scan(
		&v.SkuID, &v.SKU, &v.ProductID, &v.Color, &v.Size,
		&v.ColorCode, &v.EditionCode, &v.SizeCode,
		&v.SellingPrice, &v.Price, &v.Status, &imageURLs, &createdAt,
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

func (s *ProductSearchStore) parentSKUCodes(ctx context.Context, productID string) (brandCode, styleCode string, err error) {
	var bc, sc *string
	err = s.pool.QueryRow(ctx,
		`SELECT brand_code, style_code FROM products WHERE id = $1`, productID,
	).Scan(&bc, &sc)
	if err != nil {
		return "", "", err
	}
	if bc != nil {
		brandCode = *bc
	}
	if sc != nil {
		styleCode = *sc
	}
	return brandCode, styleCode, nil
}

func (s *ProductSearchStore) nextVariantSKU(ctx context.Context, productID, brandCode, styleCode string, v *domain.Variant) (string, error) {
	base := domain.ComposeVariantSKU(productID, brandCode, styleCode, v)

	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM product_variants WHERE sku = $1)`, base).Scan(&exists)
	if err != nil {
		return "", err
	}
	if !exists {
		return base, nil
	}
	if brandCode != "" && styleCode != "" && v.ColorCode != "" {
		return "", fmt.Errorf("duplicate sku %s: same brand/style/color/edition/size already exists", base)
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

	brandCode, styleCode, err := s.parentSKUCodes(ctx, v.ProductID)
	if err != nil {
		return nil, fmt.Errorf("product not found: %w", err)
	}

	domain.ResolveVariantCodes(&v)
	// Do not invent master rows at runtime (Phase A). Codes must already exist
	// (seeded or created via catalog APIs). Empty edition stays NULL.

	if v.SKU == "" {
		sku, err := s.nextVariantSKU(ctx, v.ProductID, brandCode, styleCode, &v)
		if err != nil {
			return nil, err
		}
		v.SKU = sku
	}
	if v.SkuID == "" {
		v.SkuID = domain.NewSkuID()
	}

	var createdAt time.Time
	err = s.pool.QueryRow(ctx,
		`INSERT INTO product_variants (sku_id, sku, product_id, color, size, color_code, edition_code, size_code, selling_price, price, status, image_urls)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 RETURNING created_at`,
		v.SkuID, v.SKU, v.ProductID, v.Color, v.Size,
		nullEmpty(v.ColorCode), nullEmpty(v.EditionCode), nullEmpty(v.SizeCode),
		v.SellingPrice, v.Price, v.Status, toTextArray(v.ImageURLs),
	).Scan(&createdAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, fmt.Errorf("%w: %s", domain.ErrMasterNotFound, pgErr.Message)
		}
		return nil, err
	}
	v.CreatedAt = createdAt.Format(time.RFC3339)
	return &v, nil
}

// UpdateVariant updates a variant by its (immutable) sku. sku_id and sku are never
// rewritten — codes may be filled when previously blank, but the human sku stays stable.
func (s *ProductSearchStore) UpdateVariant(v domain.Variant) (*domain.Variant, error) {
	var createdAt time.Time
	err := s.pool.QueryRow(context.Background(),
		`UPDATE product_variants
		 SET color=$2, size=$3, color_code=$4, edition_code=$5, size_code=$6,
		     selling_price=$7, price=$8, status=$9, image_urls=$10
		 WHERE sku=$1
		 RETURNING sku_id, product_id, created_at`,
		v.SKU, v.Color, v.Size,
		nullEmpty(v.ColorCode), nullEmpty(v.EditionCode), nullEmpty(v.SizeCode),
		v.SellingPrice, v.Price, v.Status, toTextArray(v.ImageURLs),
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
