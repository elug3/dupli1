package ports

import (
	"context"
	"errors"
)

// ErrReservationClosed is returned when a reservation was already committed or released.
var ErrReservationClosed = errors.New("reservation is not active")

// ErrReservationAlreadyCommitted is returned when commit is retried on a committed reservation.
var ErrReservationAlreadyCommitted = errors.New("reservation already committed")

// ErrReservationAlreadyReleased is returned when commit or ship targets a released reservation.
var ErrReservationAlreadyReleased = errors.New("reservation already released")

// StockItem is a line to reserve/commit against product stock.
type StockItem struct {
	SkuID    string `json:"sku_id,omitempty"`
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

// StockClient reserves and finalizes stock via the product service
// (routes under /api/v1/products/inventory/* — product owns stock after the inventory merge;
// legacy /api/v1/inventory/* remains aliased).
type StockClient interface {
	Reserve(ctx context.Context, orderID string, items []StockItem) (string, error)
	CommitReservation(ctx context.Context, reservationID string) error
	ReleaseReservation(ctx context.Context, reservationID string) error
}
