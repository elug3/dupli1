package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidOrder          = errors.New("invalid order")
	ErrInvalidTransition     = errors.New("invalid order status transition")
	ErrPaymentAmountMismatch = errors.New("payment amount does not match order total")
)

const DefaultPaymentTTL = 5 * time.Minute

type OrderStatus string

const (
	StatusPending   OrderStatus = "pending"
	StatusPaid      OrderStatus = "paid"
	StatusInTransit OrderStatus = "in_transit"
	StatusFulfilled OrderStatus = "fulfilled"
	StatusCanceled  OrderStatus = "canceled"
)

type OrderItem struct {
	SkuID          string `json:"sku_id,omitempty"`
	SKU            string `json:"sku"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"` // whole KRW won
}

type Order struct {
	ID            string      `json:"id"`
	CustomerID    string      `json:"customer_id"`
	ReservationID string      `json:"reservation_id"`
	Items         []OrderItem `json:"items"`
	Status        OrderStatus `json:"status"`
	CouponCode    string      `json:"coupon_code,omitempty"`
	SubtotalCents int64       `json:"subtotal_cents"`
	DiscountCents int64       `json:"discount_cents"`
	TotalCents    int64       `json:"total_cents"`
	PaymentID     string      `json:"payment_id,omitempty"`
	PaidAt        *time.Time  `json:"paid_at,omitempty"`
	PaymentDueAt  time.Time   `json:"payment_due_at"`
	ShippedBy     string      `json:"shipped_by,omitempty"`
	ShippedAt     *time.Time  `json:"shipped_at,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

func NewOrder(id, customerID, reservationID string, items []OrderItem, couponCode string, discountCents int64, now time.Time) (*Order, error) {
	id = strings.TrimSpace(id)
	customerID = strings.TrimSpace(customerID)
	reservationID = strings.TrimSpace(reservationID)
	if id == "" || customerID == "" || reservationID == "" {
		return nil, ErrInvalidOrder
	}

	copiedItems := make([]OrderItem, len(items))
	var subtotal int64
	for i, item := range items {
		item.SkuID = strings.TrimSpace(item.SkuID)
		item.SKU = strings.ToUpper(strings.TrimSpace(item.SKU))
		if (item.SKU == "" && item.SkuID == "") || item.Quantity <= 0 || item.UnitPriceCents < 0 {
			return nil, ErrInvalidOrder
		}
		subtotal += int64(item.Quantity) * item.UnitPriceCents
		copiedItems[i] = item
	}
	if len(copiedItems) == 0 {
		return nil, ErrInvalidOrder
	}
	if discountCents < 0 || discountCents > subtotal {
		return nil, ErrInvalidOrder
	}

	return &Order{
		ID:            id,
		CustomerID:    customerID,
		ReservationID: reservationID,
		Items:         copiedItems,
		Status:        StatusPending,
		CouponCode:    strings.ToUpper(strings.TrimSpace(couponCode)),
		SubtotalCents: subtotal,
		DiscountCents: discountCents,
		TotalCents:    subtotal - discountCents,
		PaymentDueAt:  now.Add(DefaultPaymentTTL),
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func (o *Order) MarkPaid(paymentID string, amountCents int64, now time.Time) error {
	if o.Status != StatusPending {
		return ErrInvalidTransition
	}
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return ErrInvalidOrder
	}
	if amountCents != o.TotalCents {
		return ErrPaymentAmountMismatch
	}
	o.Status = StatusPaid
	o.PaymentID = paymentID
	o.PaidAt = &now
	o.UpdatedAt = now
	return nil
}

func (o *Order) Ship(shippedBy string, now time.Time) error {
	if o.Status != StatusPaid {
		return ErrInvalidTransition
	}
	shippedBy = strings.TrimSpace(shippedBy)
	if shippedBy == "" {
		return ErrInvalidOrder
	}
	o.Status = StatusInTransit
	o.ShippedBy = shippedBy
	o.ShippedAt = &now
	o.UpdatedAt = now
	return nil
}

func (o *Order) Cancel(now time.Time) error {
	if o.Status != StatusPending && o.Status != StatusPaid {
		return ErrInvalidTransition
	}
	o.Status = StatusCanceled
	o.UpdatedAt = now
	return nil
}

func (o *Order) Fulfill(now time.Time) error {
	if o.Status != StatusInTransit {
		return ErrInvalidTransition
	}
	o.Status = StatusFulfilled
	o.UpdatedAt = now
	return nil
}

func (o *Order) IsPaymentExpired(now time.Time) bool {
	return o.Status == StatusPending && now.After(o.PaymentDueAt)
}
