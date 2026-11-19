package ports

import (
	"context"
	"errors"

	"github.com/elug3/dupli1/payment/pkg/domain"
)

type CheckoutSessionInput struct {
	OrderID    string
	PaymentID  string
	AmountWon  int64
	Currency   string
	CustomerID string
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
// payment. AmountWon is the amount to cancel, not the original total —
// providers that support partial cancel refund exactly this much.
type CancelPaymentInput struct {
	// ProviderRef is the provider's own transaction id for the original
	// approval (NANO: tranNo, captured from the payment callback).
	ProviderRef string
	// PaymentID is echoed to the provider for reconciliation (NANO: compOrderNo).
	PaymentID string
	AmountWon int64
	Currency  string
}

// CancelPaymentResult reports what the provider actually canceled. A provider
// that does not report a remaining balance sets RemainingKnown false, leaving
// the caller's own accounting authoritative.
type CancelPaymentResult struct {
	CanceledAmountWon int64
	RemainingWon      int64
	RemainingKnown    bool
	// ProviderRef is the provider's id for the original transaction, echoed
	// back on the cancel response (NANO: apprTranNo).
	ProviderRef string
	// CanceledAt is the provider's own cancel timestamp, when it reports one.
	CanceledAt string
}

// ErrCardApprovalUnconfirmed means a key-in approval was sent to the PG and
// no answer came back that says what happened (timeout, transport error,
// unreadable response). The card may have been charged, so the payment is
// held for a person to check rather than offered for another try.
var ErrCardApprovalUnconfirmed = errors.New("card approval outcome unknown")

// CardDetails is what the shopper typed for key-in; see domain.CardDetails.
type CardDetails = domain.CardDetails

// CardApprovalInput asks the PG to charge a typed card for one payment.
type CardApprovalInput struct {
	PaymentID  string
	OrderID    string
	AmountWon  int64
	OrderName  string
	OrderTel   string
	OrderEmail string
	GoodsName  string
	Card       CardDetails
}

// CardApprovalResult is the PG's parsed answer. Approved is false for a
// decline, with ResultCode/ResultMsg saying why.
type CardApprovalResult struct {
	Approved    bool
	ResultCode  string
	ResultMsg   string
	ShopCode    string
	ReqPayAmt   string
	TranNo      string
	ApprNo      string
	CardMasked  string
	CardName    string
	CompOrderNo string
}

// CardApprover charges a typed card (NANO 수기결제). Optional: a deployment
// without it keeps the redirect checkout of CheckoutProvider.
type CardApprover interface {
	ApproveCard(ctx context.Context, input CardApprovalInput) (*CardApprovalResult, error)
}
