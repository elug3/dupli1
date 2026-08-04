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
	ID              string
	CustomerID      string
	Status          string
	TotalCents      int64
	RecipientName   string
	RecipientPhone  string
	ShippingAddress ShippingAddress
}

// ShippingAddress mirrors order fulfillment snapshot fields for PG adapters.
type ShippingAddress struct {
	PostalCode   string
	AddressLine1 string
	AddressLine2 string
	City         string
	Province     string
}

type OrderClient interface {
	GetOrder(ctx context.Context, bearerToken, orderID string) (*OrderSummary, error)
}
