package domain

import (
	"errors"
	"strings"
)

// ErrInvalidPaymentMethod is returned for a payment method order does not
// price. The ids mirror payment's own (payment/pkg/domain).
var ErrInvalidPaymentMethod = errors.New("invalid payment method")

// Payment methods an order can be priced for.
const (
	PaymentMethodCreditCard = "credit_card"
	PaymentMethodBypass     = "bypass"
)

// NormalizePaymentMethod returns the canonical method. An empty method is a
// card payment, because that is what a shopper pays with unless staff mark the
// order paid another way.
func NormalizePaymentMethod(method string) (string, error) {
	m := strings.ToLower(strings.TrimSpace(method))
	switch m {
	case "":
		return PaymentMethodCreditCard, nil
	case PaymentMethodCreditCard, PaymentMethodBypass:
		return m, nil
	default:
		return "", ErrInvalidPaymentMethod
	}
}

// ApplyPaymentMethod records how a new order will be paid. It does not change
// what the order costs: card payments carried a 10% surcharge until SER-110
// removed it (2026-11-20), so every new order has CardSurchargeWon 0 and a
// card order costs the same as any other. Orders placed before keep the
// surcharge they were charged.
//
// It records a pending order's method exactly once, at creation; a second
// call is refused.
func (o *Order) ApplyPaymentMethod(method string) error {
	if o.PaymentMethod != "" || o.Status != StatusPending {
		return ErrInvalidOrder
	}
	m, err := NormalizePaymentMethod(method)
	if err != nil {
		return err
	}
	o.PaymentMethod = m
	o.CardSurchargeWon = 0
	return nil
}
