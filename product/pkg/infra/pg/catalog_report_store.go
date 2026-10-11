package pg

import (
	"context"

	"github.com/elug3/dupli1/product/pkg/domain"
)

// CatalogCounts implements ports.CatalogReportStore: one row per brand,
// category, subcategory and status, counting products, every SKU of them and
// the stock on those SKUs.
func (s *ProductSearchStore) CatalogCounts(ctx context.Context) ([]domain.CatalogCount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT COALESCE(p.brand_code, ''), p.brand, p.category, p.sub_category, p.status,
		       COUNT(*),
		       COALESCE(SUM(v.skus), 0),
		       COALESCE(SUM(v.units), 0),
		       COALESCE(SUM(v.reserved), 0)
		  FROM products p
		  LEFT JOIN (
		        SELECT pv.product_id,
		               COUNT(*) AS skus,
		               COALESCE(SUM(si.quantity), 0) AS units,
		               COALESCE(SUM(si.reserved), 0) AS reserved
		          FROM product_variants pv
		          LEFT JOIN stock_items si ON si.sku_id = pv.sku_id
		         GROUP BY pv.product_id
		       ) v ON v.product_id = p.id
		 GROUP BY 1, 2, 3, 4, 5`)
	if err != nil {
		return nil, wrapDB("catalog counts", err)
	}
	defer rows.Close()

	var out []domain.CatalogCount
	for rows.Next() {
		var c domain.CatalogCount
		if err := rows.Scan(&c.BrandCode, &c.Brand, &c.Category, &c.SubCategory, &c.Status,
			&c.Products, &c.SKUs, &c.Units, &c.Reserved); err != nil {
			return nil, wrapDB("catalog counts", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapDB("catalog counts", err)
	}
	return out, nil
}
