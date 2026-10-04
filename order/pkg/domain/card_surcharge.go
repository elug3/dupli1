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
// order paid another way — so leaving the field out can never skip the card
// surcharge.
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

// CardSurchargeWon is rateBps basis points (1000 = 10%) of baseWon, rounded
// down to the whole won so the customer is never charged a fraction up.
func CardSurchargeWon(baseWon, rateBps int64) int64 {
	if baseWon <= 0 || rateBps <= 0 {
		return 0
	}
	return baseWon * rateBps / 10000
}

// ApplyPaymentMethod records how a new order will be paid and, for a card,
// adds the card surcharge on top of everything else it costs: the goods after
// every discount, plus delivery. The surcharge is part of TotalWon, so payment
// charges it and a refund returns it without either knowing it exists.
//
// It prices a pending order exactly once, at creation; a second call is
// refused rather than stacking a second surcharge.
func (o *Order) ApplyPaymentMethod(method string, surchargeBps int64) error {
	if o.PaymentMethod != "" || o.Status != StatusPending {
		return ErrInvalidOrder
	}
	m, err := NormalizePaymentMethod(method)
	if err != nil {
		return err
	}
	o.PaymentMethod = m
	o.CardSurchargeWon = 0
	if m == PaymentMethodCreditCard {
		o.CardSurchargeWon = CardSurchargeWon(o.TotalWon, surchargeBps)
		o.TotalWon += o.CardSurchargeWon
	}
	return nil
}
