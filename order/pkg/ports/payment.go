package ports

import (
	"context"
	"errors"
	"strings"
)

var (
	// ErrPaymentUnavailable means the payment service could not be reached.
	ErrPaymentUnavailable = errors.New("payment service unavailable")
	// ErrPaymentRefundRejected means the PG refused the refund (order stays paid).
	ErrPaymentRefundRejected = errors.New("payment provider rejected the refund")
	// ErrPaymentForbidden means the caller cannot refund (missing payment.cancel).
	ErrPaymentForbidden = errors.New("forbidden: insufficient permission to refund payment")
	// ErrPaymentUnauthorized means the payment service rejected the Bearer token.
	ErrPaymentUnauthorized = errors.New("unauthorized")
)

type refundOperatorKey struct{}

// WithRefundOperator records which staff user asked for a paid-order cancel.
// The refund itself always runs as the order service account (payment.cancel);
// the operator only goes into the refund reason, so the payment row still shows
// who approved it.
func WithRefundOperator(ctx context.Context, userID string) context.Context {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ctx
	}
	return context.WithValue(ctx, refundOperatorKey{}, userID)
}

// RefundOperator returns the user ID stashed by WithRefundOperator, or "".
func RefundOperator(ctx context.Context) string {
	userID, _ := ctx.Value(refundOperatorKey{}).(string)
	return userID
}

// PaymentClient refunds a captured payment at the payment service (NANO / Bypass).
type PaymentClient interface {
	// CancelPayment requests a full refund of paymentID. idempotencyKey makes a
	// retry of the same order cancel a no-op at the payment service.
	CancelPayment(ctx context.Context, paymentID, idempotencyKey string) error
}
