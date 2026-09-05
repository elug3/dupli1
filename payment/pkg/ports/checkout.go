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
	Provider    string // domain provider id (nano, …)
	ProviderRef string
	CheckoutURL string
}

type CheckoutProvider interface {
	CreateSession(ctx context.Context, input CheckoutSessionInput) (*CheckoutSessionResult, error)
	// CancelPayment cancels (refunds) a captured payment at the provider.
	// Implementations that cannot cancel return ErrCancelUnsupported; a
	// provider that rejects the cancel returns ErrCancelRejected so the caller
	// can leave local state untouched.
	CancelPayment(ctx context.Context, input CancelPaymentInput) (*CancelPaymentResult, error)
}

// CancelPaymentInput asks the PG to cancel (refund) part or all of a captured
// payment. AmountCents is the amount to cancel, not the original total —
// providers that support partial cancel refund exactly this much.
type CancelPaymentInput struct {
	// ProviderRef is the provider's own transaction id for the original
	// approval (NANO: tranNo, captured from the payment callback).
	ProviderRef string
	// PaymentID is echoed to the provider for reconciliation (NANO: compOrderNo).
	PaymentID   string
	AmountCents int64
	Currency    string
}

// CancelPaymentResult reports what the provider actually canceled. A provider
// that does not report a remaining balance sets RemainingKnown false, leaving
// the caller's own accounting authoritative.
type CancelPaymentResult struct {
	CanceledAmountCents int64
	RemainingCents      int64
	RemainingKnown      bool
	// ProviderRef is the provider's id for the original transaction, echoed
	// back on the cancel response (NANO: apprTranNo).
	ProviderRef string
	// CanceledAt is the provider's own cancel timestamp, when it reports one.
	CanceledAt string
}
