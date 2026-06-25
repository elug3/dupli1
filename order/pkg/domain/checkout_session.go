package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidCheckoutSession = errors.New("invalid checkout session")
	ErrSessionNotOpen         = errors.New("checkout session is not open")
	ErrSessionExpired         = errors.New("checkout session has expired")
	ErrEmptyCheckout          = errors.New("checkout session has no items")
)

type CheckoutSessionStatus string

const (
	CheckoutStatusOpen      CheckoutSessionStatus = "open"
	CheckoutStatusCompleted CheckoutSessionStatus = "completed"
	CheckoutStatusExpired   CheckoutSessionStatus = "expired"
)

const DefaultCheckoutTTL = 30 * time.Minute

type CheckoutSession struct {
	ID            string                `json:"id"`
	CustomerID    string                `json:"customer_id"`
	Items         []OrderItem           `json:"items"`
	Status        CheckoutSessionStatus `json:"status"`
	CouponCode    string                `json:"coupon_code,omitempty"`
	SubtotalCents int64                 `json:"subtotal_cents"`
	DiscountCents int64                 `json:"discount_cents"`
	TotalCents    int64                 `json:"total_cents"`
	OrderID       string                `json:"order_id,omitempty"`
	ExpiresAt     time.Time             `json:"expires_at"`
	CreatedAt     time.Time             `json:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
}

func NewCheckoutSession(id, customerID string, now time.Time, ttl time.Duration) (*CheckoutSession, error) {
	id = strings.TrimSpace(id)
	customerID = strings.TrimSpace(customerID)
	if id == "" || customerID == "" {
		return nil, ErrInvalidCheckoutSession
	}
	if ttl <= 0 {
		ttl = DefaultCheckoutTTL
	}

	return &CheckoutSession{
		ID:         id,
		CustomerID: customerID,
		Items:      []OrderItem{},
		Status:     CheckoutStatusOpen,
		ExpiresAt:  now.Add(ttl),
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

func (s *CheckoutSession) EnsureOpen(now time.Time) error {
	if s.Status == CheckoutStatusCompleted {
		return ErrSessionNotOpen
	}
	if s.Status == CheckoutStatusExpired || now.After(s.ExpiresAt) {
		s.Status = CheckoutStatusExpired
		return ErrSessionExpired
	}
	return nil
}

func (s *CheckoutSession) SetItems(items []OrderItem, now time.Time) error {
	if err := s.EnsureOpen(now); err != nil {
		return err
	}

	copied := make([]OrderItem, len(items))
	for i, item := range items {
		item.SKU = strings.ToUpper(strings.TrimSpace(item.SKU))
		if item.SKU == "" || item.Quantity <= 0 || item.UnitPriceCents < 0 {
			return ErrInvalidCheckoutSession
		}
		copied[i] = item
	}

	s.Items = copied
	s.recalculateTotals()
	s.UpdatedAt = now
	return nil
}

func (s *CheckoutSession) UpsertItem(item OrderItem, now time.Time) error {
	if err := s.EnsureOpen(now); err != nil {
		return err
	}

	item.SKU = strings.ToUpper(strings.TrimSpace(item.SKU))
	if item.SKU == "" || item.Quantity <= 0 || item.UnitPriceCents < 0 {
		return ErrInvalidCheckoutSession
	}

	for i, existing := range s.Items {
		if existing.SKU == item.SKU {
			s.Items[i] = item
			s.recalculateTotals()
			s.UpdatedAt = now
			return nil
		}
	}

	s.Items = append(s.Items, item)
	s.recalculateTotals()
	s.UpdatedAt = now
	return nil
}

func (s *CheckoutSession) RemoveItem(sku string, now time.Time) error {
	if err := s.EnsureOpen(now); err != nil {
		return err
	}

	sku = strings.ToUpper(strings.TrimSpace(sku))
	if sku == "" {
		return ErrInvalidCheckoutSession
	}

	filtered := s.Items[:0]
	for _, item := range s.Items {
		if item.SKU != sku {
			filtered = append(filtered, item)
		}
	}
	s.Items = filtered
	s.recalculateTotals()
	s.UpdatedAt = now
	return nil
}

func (s *CheckoutSession) ApplyCoupon(code string, discountFraction float64, now time.Time) error {
	if err := s.EnsureOpen(now); err != nil {
		return err
	}

	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" || discountFraction <= 0 || discountFraction >= 1 {
		return ErrInvalidCheckoutSession
	}

	s.CouponCode = code
	s.recalculateTotalsWithDiscount(discountFraction)
	s.UpdatedAt = now
	return nil
}

func (s *CheckoutSession) ClearCoupon(now time.Time) error {
	if err := s.EnsureOpen(now); err != nil {
		return err
	}

	s.CouponCode = ""
	s.recalculateTotals()
	s.UpdatedAt = now
	return nil
}

func (s *CheckoutSession) Complete(orderID string, now time.Time) error {
	if err := s.EnsureOpen(now); err != nil {
		return err
	}
	if len(s.Items) == 0 {
		return ErrEmptyCheckout
	}

	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return ErrInvalidCheckoutSession
	}

	s.Status = CheckoutStatusCompleted
	s.OrderID = orderID
	s.UpdatedAt = now
	return nil
}

func (s *CheckoutSession) recalculateTotals() {
	s.recalculateTotalsWithDiscount(0)
}

func (s *CheckoutSession) recalculateTotalsWithDiscount(discountFraction float64) {
	var subtotal int64
	for _, item := range s.Items {
		subtotal += int64(item.Quantity) * item.UnitPriceCents
	}

	s.SubtotalCents = subtotal
	if discountFraction > 0 && s.CouponCode != "" {
		s.DiscountCents = int64(float64(subtotal) * discountFraction)
	} else {
		s.DiscountCents = 0
	}
	s.TotalCents = subtotal - s.DiscountCents
}
