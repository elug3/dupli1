package ports

import "context"

type InventoryClient interface {
	GetAvailableQty(ctx context.Context, sku string) (int, error)
}
