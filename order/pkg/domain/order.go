package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidOrder      = errors.New("invalid order")
	ErrInvalidTransition = errors.New("invalid order status transition")
)

type OrderStatus string

const (
	StatusPending   OrderStatus = "pending"
	StatusConfirmed OrderStatus = "confirmed"
	StatusCanceled  OrderStatus = "canceled"
	StatusFulfilled OrderStatus = "fulfilled"
)

type OrderItem struct {
	SKU            string `json:"sku"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
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
		item.SKU = strings.ToUpper(strings.TrimSpace(item.SKU))
		if item.SKU == "" || item.Quantity <= 0 || item.UnitPriceCents < 0 {
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
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func (o *Order) Confirm(now time.Time) error {
	if o.Status != StatusPending {
		return ErrInvalidTransition
	}
	o.Status = StatusConfirmed
	o.UpdatedAt = now
	return nil
}

func (o *Order) Cancel(now time.Time) error {
	if o.Status != StatusPending {
		return ErrInvalidTransition
	}
	o.Status = StatusCanceled
	o.UpdatedAt = now
	return nil
}

func (o *Order) Fulfill(now time.Time) error {
	if o.Status != StatusConfirmed {
		return ErrInvalidTransition
	}
	o.Status = StatusFulfilled
	o.UpdatedAt = now
	return nil
}
