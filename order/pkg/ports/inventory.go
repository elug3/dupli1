package ports

import "context"

type InventoryItem struct {
	SkuID    string `json:"sku_id,omitempty"`
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

type InventoryClient interface {
	Reserve(ctx context.Context, orderID string, items []InventoryItem) (string, error)
	CommitReservation(ctx context.Context, reservationID string) error
	ReleaseReservation(ctx context.Context, reservationID string) error
}
