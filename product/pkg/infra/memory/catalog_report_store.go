package memory

import (
	"context"

	"github.com/elug3/dupli1/product/pkg/domain"
)

// CatalogCounts implements ports.CatalogReportStore, mirroring the pg query:
// one row per brand, category, subcategory and status.
func (s *ProductStore) CatalogCounts(ctx context.Context) ([]domain.CatalogCount, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	type key struct{ brandCode, brand, category, sub, status string }
	groups := map[key]*domain.CatalogCount{}
	var order []key
	for _, p := range s.Products {
		k := key{p.BrandCode, p.Brand, p.Category, p.SubCategory, p.Status}
		c, ok := groups[k]
		if !ok {
			c = &domain.CatalogCount{BrandCode: p.BrandCode, Brand: p.Brand, Category: p.Category, SubCategory: p.SubCategory, Status: p.Status}
			groups[k] = c
			order = append(order, k)
		}
		c.Products++
		for _, v := range s.variantsFor(p.ID) {
			c.SKUs++
			if item := s.stockFor(v.SkuID); item != nil {
				c.Units += item.Quantity
				c.Reserved += item.Reserved
			}
		}
	}
	out := make([]domain.CatalogCount, 0, len(order))
	for _, k := range order {
		out = append(out, *groups[k])
	}
	return out, nil
}

func (s *ProductStore) stockFor(skuID string) *domain.StockItem {
	if s.inventory == nil || skuID == "" {
		return nil
	}
	s.inventory.mu.RLock()
	defer s.inventory.mu.RUnlock()
	if item, ok := s.inventory.items[skuID]; ok {
		copied := *item
		return &copied
	}
	return nil
}
