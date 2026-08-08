package ports

import "context"

type CheckoutSessionInput struct {
	OrderID     string
	PaymentID   string
	AmountCents int64
	Currency    string
	CustomerID  string
	// Order snapshot fields for certified PG requests (NANO).
	OrderName  string
	OrderTel   string
	OrderEmail string
	GoodsName  string
}

type CheckoutSessionResult struct {
	Provider    string // domain provider id (dev, nano, …)
	ProviderRef string
	CheckoutURL string
}

type CheckoutProvider interface {
	CreateSession(ctx context.Context, input CheckoutSessionInput) (*CheckoutSessionResult, error)
}
