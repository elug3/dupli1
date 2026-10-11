package ports

import (
	"context"

	"github.com/elug3/dupli1/product/pkg/domain"
)

// CatalogReportStore counts the catalog grouped by brand, category,
// subcategory and status, with each group's SKUs and stock.
type CatalogReportStore interface {
	CatalogCounts(ctx context.Context) ([]domain.CatalogCount, error)
}
