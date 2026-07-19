package ports

import "context"

// StockItem is a line to reserve/commit against product stock.
type StockItem struct {
	SkuID    string `json:"sku_id,omitempty"`
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

// StockClient reserves and finalizes stock via the product service
// (routes under /api/v1/inventory/* — product owns stock after the inventory merge).
type StockClient interface {
	Reserve(ctx context.Context, orderID string, items []StockItem) (string, error)
	CommitReservation(ctx context.Context, reservationID string) error
	ReleaseReservation(ctx context.Context, reservationID string) error
}
