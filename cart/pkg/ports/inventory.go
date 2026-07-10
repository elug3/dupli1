package ports

import "context"

type InventoryClient interface {
	GetAvailableQty(ctx context.Context, sku string) (int, error)
	GetAvailableQtyBySkuID(ctx context.Context, skuID string) (int, error)
}
