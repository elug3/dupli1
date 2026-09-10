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

type paymentBearerKey struct{}

// WithPaymentBearer stores the caller's access token so a paid-order cancel can
// refund at the payment service as that operator (payment.cancel).
func WithPaymentBearer(ctx context.Context, token string) context.Context {
	token = trimBearer(token)
	if token == "" {
		return ctx
	}
	return context.WithValue(ctx, paymentBearerKey{}, token)
}

// PaymentBearer returns the operator access token stashed by WithPaymentBearer.
func PaymentBearer(ctx context.Context) string {
	token, _ := ctx.Value(paymentBearerKey{}).(string)
	return token
}

func trimBearer(token string) string {
	token = strings.TrimSpace(token)
	if len(token) >= 7 && strings.EqualFold(token[:7], "bearer ") {
		return strings.TrimSpace(token[7:])
	}
	return token
}

// PaymentClient refunds a captured payment at the payment service (NANO / Bypass).
type PaymentClient interface {
	// CancelPayment requests a full refund of paymentID. idempotencyKey makes a
	// retry of the same order cancel a no-op at the payment service.
	CancelPayment(ctx context.Context, paymentID, idempotencyKey string) error
}
