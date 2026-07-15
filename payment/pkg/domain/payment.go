package domain

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidPayment = errors.New("invalid payment")

type PaymentStatus string

const (
	StatusRequiresPayment PaymentStatus = "requires_payment"
	StatusSucceeded       PaymentStatus = "succeeded"
	StatusFailed          PaymentStatus = "failed"
	StatusCanceled        PaymentStatus = "canceled"
	StatusExpired         PaymentStatus = "expired"
)

const DefaultPaymentTTL = 5 * time.Minute

// DefaultCurrency is the storefront / payment currency when none is supplied.
const DefaultCurrency = "krw"

type Payment struct {
	ID            string        `json:"id"`
	OrderID       string        `json:"order_id"`
	CustomerID    string        `json:"customer_id"`
	AmountCents   int64         `json:"amount_cents"`
	Currency      string        `json:"currency"`
	Status        PaymentStatus `json:"status"`
	Provider      string        `json:"provider"`
	ProviderRef   string        `json:"provider_ref"`
	CheckoutURL   string        `json:"checkout_url,omitempty"`
	IdempotencyKey string       `json:"-"`
	ExpiresAt     time.Time     `json:"expires_at"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

func NewPayment(id, orderID, customerID string, amountCents int64, currency, provider, providerRef, checkoutURL string, now time.Time) (*Payment, error) {
	if id == "" || orderID == "" || customerID == "" || amountCents <= 0 {
		return nil, ErrInvalidPayment
	}
	if currency == "" {
		currency = DefaultCurrency
	}
	return &Payment{
		ID:          id,
		OrderID:     orderID,
		CustomerID:  customerID,
		AmountCents: amountCents,
		Currency:    strings.ToLower(currency),
		Status:      StatusRequiresPayment,
		Provider:    provider,
		ProviderRef: providerRef,
		CheckoutURL: checkoutURL,
		ExpiresAt:   now.Add(DefaultPaymentTTL),
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (p *Payment) MarkSucceeded(now time.Time) {
	p.Status = StatusSucceeded
	p.UpdatedAt = now
}
