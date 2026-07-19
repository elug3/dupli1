package ports

import (
	"context"
	"errors"
)

var (
	ErrOrderNotFound       = errors.New("order not found")
	ErrOrderNotPending     = errors.New("order is not pending")
	ErrOrderForbidden      = errors.New("order does not belong to customer")
	ErrPaymentForbidden    = errors.New("payment method not allowed")
	ErrMethodUnavailable   = errors.New("payment method not available")
)

type OrderSummary struct {
	ID           string
	CustomerID   string
	Status       string
	TotalCents   int64
}

type OrderClient interface {
	GetOrder(ctx context.Context, bearerToken, orderID string) (*OrderSummary, error)
}
