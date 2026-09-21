package service

import (
	"context"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

// StorePromotionCatalog answers catalog questions about cart lines from the
// product store.
//
// It sits in the service layer rather than beside a driver because it is the
// same for every store: both the Postgres and the in-memory product stores
// satisfy ports.ProductStore, and the lookup is variant → parent either way.
type StorePromotionCatalog struct {
	store ports.ProductStore
}

func NewStorePromotionCatalog(store ports.ProductStore) *StorePromotionCatalog {
	return &StorePromotionCatalog{store: store}
}

// LineAttributes resolves each ref to its catalog attributes.
//
// Lines carrying a sku id are resolved in one query; a line that has only the
// human SKU — an older cart, or a caller that never had the id — falls back to
// a lookup of its own. A ref the catalog cannot resolve comes back unfound
// rather than failing the evaluation: the line then matches no catalog
// predicate, which is the same answer as a line that genuinely does not
// qualify.
func (c *StorePromotionCatalog) LineAttributes(
	ctx context.Context,
	refs []ports.LineRef,
) ([]ports.LineCatalog, error) {
	out := make([]ports.LineCatalog, len(refs))
	if c.store == nil || len(refs) == 0 {
		return out, nil
	}

	skuIDs := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.SkuID != "" {
			skuIDs = append(skuIDs, ref.SkuID)
		}
	}

	variants := make(map[string]domain.Variant, len(refs))
	if len(skuIDs) > 0 {
		found, err := c.store.GetVariantsBySkuIDs(ctx, skuIDs)
		if err != nil {
			return nil, err
		}
		for _, v := range found {
			variants[v.SkuID] = v
		}
	}

	// One parent read per distinct product, not per line: a cart is usually
	// several variants of few parents.
	parents := make(map[string]*domain.Product)
	for i, ref := range refs {
		variant, ok := variants[ref.SkuID]
		if !ok {
			if ref.SKU == "" {
				continue
			}
			v, err := c.store.GetVariant(ctx, ref.SKU)
			if err != nil || v == nil {
				continue
			}
			variant = *v
		}

		parent, seen := parents[variant.ProductID]
		if !seen {
			p, err := c.store.GetProduct(ctx, variant.ProductID)
			if err == nil {
				parent = p
			}
			parents[variant.ProductID] = parent
		}
		if parent == nil {
			continue
		}

		out[i] = ports.LineCatalog{
			Found:     true,
			SkuID:     variant.SkuID,
			SKU:       variant.SKU,
			ProductID: parent.ID,
			Category:  parent.Category,
			BrandCode: parent.BrandCode,
			// A markdown is the parent's official price standing above what it
			// actually sells for; price lives on the parent, never the variant.
			OnSale: parent.OfficialPrice > parent.Price,
		}
	}
	return out, nil
}
