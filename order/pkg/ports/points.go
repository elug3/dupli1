package ports

import (
	"context"
	"errors"
)

var (
	// ErrPointsUnavailable means payment, which keeps the points ledger,
	// could not be asked. Checkout with points fails closed on it: an order
	// must never take points off its price that were not set aside.
	ErrPointsUnavailable = errors.New("points service unavailable")
	// ErrInsufficientPoints is a shopper asking to spend more points than
	// they hold.
	ErrInsufficientPoints = errors.New("insufficient points")
	// ErrInvalidPoints is a negative or otherwise unusable points amount.
	ErrInvalidPoints = errors.New("invalid points")
)

// PointsClient sets points aside for an order and settles them, through
// payment's internal hold routes (docs/payment-points.md). Every call is keyed
// by order id and idempotent, like the promotional-code ledger.
type PointsClient interface {
	// Reserve sets points aside for an order. ErrInsufficientPoints when the
	// customer holds fewer.
	Reserve(ctx context.Context, orderID, customerID string, points int64) error
	// Consume spends the hold once the order is paid.
	Consume(ctx context.Context, orderID string) error
	// Release gives the hold back on cancel, spent or not. An order with no
	// hold is not an error.
	Release(ctx context.Context, orderID string) error
}
