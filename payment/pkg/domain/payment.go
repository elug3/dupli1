package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/elug3/dupli1/shared/pkg/money"
)

var (
	ErrInvalidPayment = errors.New("invalid payment")
	// ErrNotCancelable is returned when a payment is in a state that cannot be
	// canceled at the PG (not succeeded, or already fully canceled).
	ErrNotCancelable = errors.New("payment is not cancelable")
	// ErrCancelAmountInvalid is returned when the requested cancel amount is
	// not positive or exceeds what is still captured.
	ErrCancelAmountInvalid = errors.New("invalid cancel amount")
)

type PaymentStatus string

const (
	StatusRequiresPayment PaymentStatus = "requires_payment"
	StatusSucceeded       PaymentStatus = "succeeded"
	StatusFailed          PaymentStatus = "failed"
	StatusCanceled        PaymentStatus = "canceled"
	StatusExpired         PaymentStatus = "expired"
)

// Payment method identifiers (API `method` field).
const (
	MethodCreditCard = "credit_card"
	MethodBypass     = "bypass"
	MethodBitcoin    = "bitcoin"
)

// Provider identifiers stored on the payment row.
const (
	ProviderBypass = "bypass"
	ProviderNano   = "nano"
)

const DefaultPaymentTTL = 5 * time.Minute

// DefaultCurrency is the only storefront / payment currency (KRW).
const DefaultCurrency = money.Currency

type Payment struct {
	ID          string        `json:"id"`
	OrderID     string        `json:"order_id"`
	CustomerID  string        `json:"customer_id"`
	AmountCents int64         `json:"amount_cents"` // whole KRW won (zero-decimal minor units)
	Currency    string        `json:"currency"`
	Status      PaymentStatus `json:"status"`
	Method      string        `json:"method"`
	Provider    string        `json:"provider"`
	ProviderRef string        `json:"provider_ref"`
	CheckoutURL string        `json:"checkout_url,omitempty"`
	CreatedBy   string        `json:"created_by,omitempty"`
	Note        string        `json:"note,omitempty"`
	// PayerName / PayerPhone are snapshotted from the order for NANO cert requests.
	// Omitted from API JSON — not needed by clients.
	PayerName      string `json:"-"`
	PayerPhone     string `json:"-"`
	PayerEmail     string `json:"-"`
	IdempotencyKey string `json:"-"`
	// CanceledAmountCents is the cumulative amount canceled at the PG. It stays
	// 0 for an untouched payment, sits between 1 and AmountCents-1 after a
	// partial cancel (Status remains succeeded), and equals AmountCents once
	// fully canceled (Status becomes canceled).
	CanceledAmountCents int64      `json:"canceled_amount_cents,omitempty"`
	CanceledAt          *time.Time `json:"canceled_at,omitempty"`
	CancelReason        string     `json:"cancel_reason,omitempty"`
	CanceledBy          string     `json:"canceled_by,omitempty"`
	// CancelIdempotencyKey is the key of the most recent applied cancel, used to
	// make a client retry of that same cancel a no-op. Only the latest key is
	// kept: it guards the realistic double-submit (timeout then retry), not an
	// arbitrary replay of an older partial cancel.
	CancelIdempotencyKey string    `json:"-"`
	ExpiresAt            time.Time `json:"expires_at"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func (p *Payment) MarkFailed(now time.Time) {
	p.Status = StatusFailed
	p.UpdatedAt = now
}

func NewPayment(id, orderID, customerID string, amountCents int64, currency, provider, providerRef, checkoutURL string, now time.Time) (*Payment, error) {
	if id == "" || orderID == "" || customerID == "" || amountCents <= 0 {
		return nil, ErrInvalidPayment
	}
	normalized, err := money.NormalizeCurrency(currency)
	if err != nil {
		return nil, ErrInvalidPayment
	}
	return &Payment{
		ID:          id,
		OrderID:     orderID,
		CustomerID:  customerID,
		AmountCents: amountCents,
		Currency:    normalized,
		Status:      StatusRequiresPayment,
		Method:      MethodCreditCard,
		Provider:    provider,
		ProviderRef: providerRef,
		CheckoutURL: checkoutURL,
		ExpiresAt:   now.Add(DefaultPaymentTTL),
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// NormalizeMethod returns a canonical method or ErrInvalidPayment.
// Empty method defaults to credit_card.
func NormalizeMethod(method string) (string, error) {
	m := strings.TrimSpace(strings.ToLower(method))
	if m == "" {
		return MethodCreditCard, nil
	}
	switch m {
	case MethodCreditCard, MethodBypass, MethodBitcoin:
		return m, nil
	default:
		return "", ErrInvalidPayment
	}
}

func (p *Payment) MarkSucceeded(now time.Time) {
	p.Status = StatusSucceeded
	p.UpdatedAt = now
}

// RemainingCancelableCents is the amount still captured and therefore still
// cancelable at the PG.
func (p *Payment) RemainingCancelableCents() int64 {
	remaining := p.AmountCents - p.CanceledAmountCents
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Cancelable reports whether a cancel may be attempted against the PG.
// Only a succeeded payment with a positive remaining balance qualifies —
// requires_payment is abandoned rather than canceled, and failed/expired
// never captured money.
func (p *Payment) Cancelable() bool {
	return p.Status == StatusSucceeded && p.RemainingCancelableCents() > 0
}

// ValidateCancel checks a requested cancel without mutating the payment, so
// callers can reject bad input before spending a PG round trip.
// amountCents must be positive and at most the remaining balance.
func (p *Payment) ValidateCancel(amountCents int64) error {
	if !p.Cancelable() {
		return ErrNotCancelable
	}
	if amountCents <= 0 || amountCents > p.RemainingCancelableCents() {
		return ErrCancelAmountInvalid
	}
	return nil
}

// ApplyCancel records a cancel that the PG has already accepted. A cancel that
// exhausts the remaining balance moves the payment to canceled; a partial
// cancel leaves it succeeded with a reduced remaining balance, matching NANO's
// remainAmt semantics (see [NANO] 수기결제 연동 API v2.5 §3).
func (p *Payment) ApplyCancel(amountCents int64, reason, canceledBy string, now time.Time) error {
	if err := p.ValidateCancel(amountCents); err != nil {
		return err
	}
	p.CanceledAmountCents += amountCents
	// Keep whatever a previous cancel recorded when this one supplies nothing:
	// a later cancel without a reason must not erase the earlier audit trail.
	// Only the most recent stated reason/actor is kept — a full per-cancel
	// history needs its own table.
	if r := strings.TrimSpace(reason); r != "" {
		p.CancelReason = r
	}
	if by := strings.TrimSpace(canceledBy); by != "" {
		p.CanceledBy = by
	}
	p.CanceledAt = &now
	if p.RemainingCancelableCents() == 0 {
		p.Status = StatusCanceled
	}
	p.UpdatedAt = now
	return nil
}
