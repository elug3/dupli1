// Package events defines the NATS subject names and payload shapes shared
// between a publishing service and its subscriber(s): order publishes
// order.* (notification subscribes), payment publishes payment.succeeded
// (order subscribes) and payment.callback_rejected (notification
// subscribes), product publishes product.* (notification
// subscribes), and auth publishes user.deleted (profile subscribes, to
// cascade-delete saved profile/address data). Each subject has exactly one
// publisher and one or more subscribers that must agree on the exact string
// and payload fields, so both are defined once here rather than redeclared
// per service.
package events

import (
	"encoding/json"
	"time"
)

// Subject names published over NATS.
const (
	OrderCreated      = "order.created"
	OrderStatusUpdate = "order.status_updated"
	OrderPaid         = "order.paid"
	PaymentSucceeded  = "payment.succeeded"
	PaymentCanceled   = "payment.canceled"
	// PaymentCallbackRejected fires when a PG callback the PG itself marked
	// approved could not be applied — the money may be gone with no paid order.
	PaymentCallbackRejected = "payment.callback_rejected"
	ProductCreated          = "product.created"
	ProductUpdated          = "product.updated"
	ProductDeleted          = "product.deleted"
	ProductImage            = "product.image_uploaded"
	UserRegistered          = "user.registered"
	UserDeleted             = "user.deleted"
	// SupportInquiryOpened fires when a shopper asks the support bot for a
	// human. Published by support, consumed by notification, which owns where
	// ops alerts go.
	SupportInquiryOpened = "support.inquiry_opened"
)

// SupportInquiry is the payload for SupportInquiryOpened — published by
// support, consumed by notification.
//
// Deliberately carries an excerpt rather than the conversation: message bodies
// are customer data, and an ops alert is not where they belong in full. The
// excerpt is what a manager needs to decide whether to pick the inquiry up; the
// rest waits behind the manager inbox.
type SupportInquiry struct {
	InquiryID string `json:"inquiry_id"`
	ChatID    string `json:"chat_id"`
	// Topic is the menu node the shopper escalated from (ord, ret, agt, …).
	Topic string `json:"topic"`
	// Language is the entry language the storefront passed, recorded but not
	// acted on while the bot is Korean-only.
	Language string `json:"language"`
	Username string `json:"username,omitempty"`
	// EntryContext is the storefront deep-link payload: a hint about the page
	// the shopper came from, never an identity and never a permission.
	EntryContext string `json:"entry_context,omitempty"`
	Excerpt      string `json:"excerpt,omitempty"`
	// ManageURL points a manager at this inquiry in the admin console.
	ManageURL string `json:"manage_url,omitempty"`
	// AfterHours marks an inquiry opened outside the service window, so the
	// alert can arrive without a ping and wait for the morning shift.
	AfterHours bool      `json:"after_hours"`
	OpenedAt   time.Time `json:"opened_at"`
	Occurred   time.Time `json:"occurred_at"`
}

// OrderItem is one line of an Order event payload.
type OrderItem struct {
	SkuID        string `json:"sku_id,omitempty"`
	SKU          string `json:"sku"`
	Quantity     int    `json:"quantity"`
	UnitPriceWon int64  `json:"unit_price_won"`
}

// Order is the payload for OrderCreated, OrderStatusUpdate, and OrderPaid —
// published by order, consumed by notification.
type Order struct {
	EventType   string `json:"event_type"`
	OrderID     string `json:"order_id"`
	CustomerID  string `json:"customer_id"`
	Status      string `json:"status"`
	SubtotalWon int64  `json:"subtotal_won"`
	DiscountWon int64  `json:"discount_won"`
	// ShippingFeeWon is the delivery charge included in TotalWon, in whole
	// KRW. Zero for orders placed before shipping fees existed, and for any
	// deployment running with delivery free.
	ShippingFeeWon int64       `json:"shipping_fee_won"`
	TotalWon       int64       `json:"total_won"`
	Items          []OrderItem `json:"items"`
	CreatedAt      time.Time   `json:"created_at"`
	Occurred       time.Time   `json:"occurred_at"`
}

func (e *Order) UnmarshalJSON(data []byte) error {
	type itemWire struct {
		SkuID         string `json:"sku_id"`
		SKU           string `json:"sku"`
		Quantity      int    `json:"quantity"`
		UnitPriceWon  *int64 `json:"unit_price_won"`
		UnitPriceKRW  *int64 `json:"unit_price_krw"`
		UnitPriceCent *int64 `json:"unit_price_cents"`
	}
	type wire struct {
		EventType      string     `json:"event_type"`
		OrderID        string     `json:"order_id"`
		CustomerID     string     `json:"customer_id"`
		Status         string     `json:"status"`
		SubtotalWon    *int64     `json:"subtotal_won"`
		SubtotalKRW    *int64     `json:"subtotal_krw"`
		DiscountWon    *int64     `json:"discount_won"`
		DiscountKRW    *int64     `json:"discount_krw"`
		ShippingFeeWon *int64     `json:"shipping_fee_won"`
		ShippingFeeKRW *int64     `json:"shipping_fee_krw"`
		TotalWon       *int64     `json:"total_won"`
		TotalKRW       *int64     `json:"total_krw"`
		Items          []itemWire `json:"items"`
		CreatedAt      time.Time  `json:"created_at"`
		Occurred       time.Time  `json:"occurred_at"`
	}
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	e.EventType = w.EventType
	e.OrderID = w.OrderID
	e.CustomerID = w.CustomerID
	e.Status = w.Status
	e.CreatedAt = w.CreatedAt
	e.Occurred = w.Occurred
	if v, ok := pickWon(w.SubtotalWon, w.SubtotalKRW, nil); ok {
		e.SubtotalWon = v
	}
	if v, ok := pickWon(w.DiscountWon, w.DiscountKRW, nil); ok {
		e.DiscountWon = v
	}
	if v, ok := pickWon(w.ShippingFeeWon, w.ShippingFeeKRW, nil); ok {
		e.ShippingFeeWon = v
	}
	if v, ok := pickWon(w.TotalWon, w.TotalKRW, nil); ok {
		e.TotalWon = v
	}
	e.Items = make([]OrderItem, len(w.Items))
	for i, item := range w.Items {
		e.Items[i] = OrderItem{SkuID: item.SkuID, SKU: item.SKU, Quantity: item.Quantity}
		if v, ok := pickWon(item.UnitPriceWon, item.UnitPriceKRW, item.UnitPriceCent); ok {
			e.Items[i].UnitPriceWon = v
		}
	}
	return nil
}

// Product is the payload for ProductCreated, ProductUpdated,
// ProductDeleted, and ProductImage — published by product, consumed by
// notification. Also reused (with a service-local subject) for product's
// unconsumed variant_created/updated/deleted events.
type Product struct {
	EventType string    `json:"event_type"`
	ProductID string    `json:"product_id"`
	SKU       string    `json:"sku,omitempty"`
	Name      string    `json:"name"`
	Brand     string    `json:"brand"`
	Category  string    `json:"category"`
	Status    string    `json:"status"`
	Price     float64   `json:"price"`
	ImageURL  string    `json:"image_url,omitempty"`
	Occurred  time.Time `json:"occurred_at"`
}

// PaymentSucceededEvent is the payload for PaymentSucceeded — published by
// payment, consumed by order.
type PaymentSucceededEvent struct {
	EventType string `json:"event_type"`
	OrderID   string `json:"order_id"`
	PaymentID string `json:"payment_id"`
	AmountWon int64  `json:"amount_won"`
}

func (e *PaymentSucceededEvent) UnmarshalJSON(data []byte) error {
	type wire struct {
		EventType   string `json:"event_type"`
		OrderID     string `json:"order_id"`
		PaymentID   string `json:"payment_id"`
		AmountWon   *int64 `json:"amount_won"`
		AmountKRW   *int64 `json:"amount_krw"`
		AmountCents *int64 `json:"amount_cents"`
	}
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	e.EventType = w.EventType
	e.OrderID = w.OrderID
	e.PaymentID = w.PaymentID
	if amount, ok := pickWon(w.AmountWon, w.AmountKRW, w.AmountCents); ok {
		e.AmountWon = amount
	}
	return nil
}

// PaymentCanceledEvent is the payload for PaymentCanceled — published by
// payment when a succeeded payment is canceled (fully or partially) at the PG.
// AmountWon is the amount canceled by this event; RemainingWon is what is
// still captured afterwards (0 on a full cancel). Order cancels a still-paid
// order on a full refund (remaining_won == 0) when payment_id matches;
// notification alerts ops. A missing remaining_won (and legacy remaining_cents)
// must not be treated as 0.
type PaymentCanceledEvent struct {
	EventType    string    `json:"event_type"`
	OrderID      string    `json:"order_id"`
	PaymentID    string    `json:"payment_id"`
	AmountWon    int64     `json:"amount_won"`
	RemainingWon int64     `json:"remaining_won"`
	Reason       string    `json:"reason,omitempty"`
	CanceledBy   string    `json:"canceled_by,omitempty"`
	Occurred     time.Time `json:"occurred_at"`
	remainingSet bool      `json:"-"`
}

// RemainingSpecified reports whether remaining_won (or the legacy
// remaining_cents alias) was present in the JSON payload. encoding/json treats
// a missing int64 as 0, which order would otherwise interpret as a full refund.
func (e PaymentCanceledEvent) RemainingSpecified() bool {
	return e.remainingSet
}

func (e *PaymentCanceledEvent) UnmarshalJSON(data []byte) error {
	type wire struct {
		EventType      string    `json:"event_type"`
		OrderID        string    `json:"order_id"`
		PaymentID      string    `json:"payment_id"`
		AmountWon      *int64    `json:"amount_won"`
		AmountKRW      *int64    `json:"amount_krw"`
		AmountCents    *int64    `json:"amount_cents"`
		RemainingWon   *int64    `json:"remaining_won"`
		RemainingKRW   *int64    `json:"remaining_krw"`
		RemainingCents *int64    `json:"remaining_cents"`
		Reason         string    `json:"reason"`
		CanceledBy     string    `json:"canceled_by"`
		Occurred       time.Time `json:"occurred_at"`
	}
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	e.EventType = w.EventType
	e.OrderID = w.OrderID
	e.PaymentID = w.PaymentID
	e.Reason = w.Reason
	e.CanceledBy = w.CanceledBy
	e.Occurred = w.Occurred
	if amount, ok := pickWon(w.AmountWon, w.AmountKRW, w.AmountCents); ok {
		e.AmountWon = amount
	}
	if remaining, ok := pickWon(w.RemainingWon, w.RemainingKRW, w.RemainingCents); ok {
		e.RemainingWon = remaining
		e.remainingSet = true
	}
	return nil
}

// pickWon prefers the canonical *_won JSON field, then leftover *_krw, then
// leftover *_cents, so in-flight outbox rows still decode after the rename.
func pickWon(won, krw, cents *int64) (int64, bool) {
	if won != nil {
		return *won, true
	}
	if krw != nil {
		return *krw, true
	}
	if cents != nil {
		return *cents, true
	}
	return 0, false
}

// PaymentCallbackRejectedEvent is the payload for PaymentCallbackRejected —
// published by payment, consumed by notification.
//
// It is emitted only when the PG reported approval (NANO resultCode 0000) and
// dupli1 still refused to mark the payment succeeded. That combination means a
// card was probably charged with no paid order behind it, so it needs a human,
// not a retry. Ordinary declines publish nothing.
//
// Every field is best-effort: a callback can be rejected precisely because it
// failed to identify a payment, so PaymentID/OrderID may be empty.
type PaymentCallbackRejectedEvent struct {
	EventType string `json:"event_type"`
	// Provider is the PG that sent the callback ("nano").
	Provider string `json:"provider"`
	// Source is which endpoint received it: "return" (shopper's browser) or
	// "webhook" (server-to-server).
	Source    string `json:"source"`
	PaymentID string `json:"payment_id,omitempty"`
	OrderID   string `json:"order_id,omitempty"`
	// Reason is the internal rejection cause — see payment's nanoReject* values.
	Reason string `json:"reason"`
	// ResultCode is the PG's own result code, retained verbatim.
	ResultCode string `json:"result_code,omitempty"`
	// ExpectedWon is the amount dupli1 holds for the payment, in whole KRW;
	// ReportedAmount is what the PG sent, unparsed, so a malformed value survives
	// into the alert instead of being flattened to 0.
	ExpectedWon    int64  `json:"expected_won,omitempty"`
	ReportedAmount string `json:"reported_amount,omitempty"`
	TranNo         string `json:"tran_no,omitempty"`
	// Detail is a short human-readable note for the alert (never a secret).
	Detail   string    `json:"detail,omitempty"`
	Occurred time.Time `json:"occurred_at"`
}

// UserRegisteredEvent is the payload for UserRegistered — published by auth,
// consumed by product, which issues a welcome promotional code to new customer
// accounts.
//
// AccountType matters to the subscriber: only "customer" accounts get one.
// Issuing to a manager or a service account would put a discount in a wallet
// nobody shops from.
type UserRegisteredEvent struct {
	EventType   string    `json:"event_type"`
	UserID      string    `json:"user_id"`
	Email       string    `json:"email"`
	AccountType string    `json:"account_type"`
	Occurred    time.Time `json:"occurred_at"`
}

// UserDeletedEvent is the payload for UserDeleted — published by auth,
// consumed by profile (which owns no foreign key to auth's users table and
// must clean up saved profile/address data itself).
type UserDeletedEvent struct {
	EventType string    `json:"event_type"`
	UserID    string    `json:"user_id"`
	Occurred  time.Time `json:"occurred_at"`
}
