package ports

import "context"

type CheckoutSessionInput struct {
	OrderID     string
	PaymentID   string
	AmountCents int64
	Currency    string
	CustomerID  string
}

type CheckoutSessionResult struct {
	ProviderRef string
	CheckoutURL string
}

type CheckoutProvider interface {
	CreateSession(ctx context.Context, input CheckoutSessionInput) (*CheckoutSessionResult, error)
}
