package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidOrder          = errors.New("invalid order")
	ErrInvalidTransition     = errors.New("invalid order status transition")
	ErrPaymentAmountMismatch = errors.New("payment amount does not match order total")
	ErrInvalidShipment       = errors.New("invalid shipment tracking")
)

const DefaultPaymentTTL = 5 * time.Minute

// ManagerConfirmationWindow is the SLA for both order confirmation after
// payment and manager approval of a post-confirm / in-transit cancel request.
const ManagerConfirmationWindow = 2 * time.Hour

// MaxCancelRequestReasonLen caps the optional customer cancel-request note.
const MaxCancelRequestReasonLen = 500

// ReasonVariantNotFound is returned when a line cannot be resolved to an
// active, sellable product variant.
const ReasonVariantNotFound = "variant_not_found"

type OrderStatus string

const (
	StatusPending   OrderStatus = "pending"
	StatusPaid      OrderStatus = "paid"
	StatusInTransit OrderStatus = "in_transit"
	StatusFulfilled OrderStatus = "fulfilled"
	StatusCanceled  OrderStatus = "canceled"
)

// UnavailableItem identifies a checkout/order line that cannot be purchased.
type UnavailableItem struct {
	SkuID  string `json:"sku_id,omitempty"`
	SKU    string `json:"sku,omitempty"`
	Reason string `json:"reason"`
}

type OrderItem struct {
	SkuID        string `json:"sku_id,omitempty"`
	SKU          string `json:"sku"`
	Quantity     int    `json:"quantity"`
	UnitPriceKRW int64  `json:"unit_price_krw"` // whole KRW won
	// ProductName and ImageURL are captured at order creation from the product catalog.
	ProductName string `json:"product_name,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	// Available is false when the variant is no longer sellable (checkout session reads).
	Available *bool `json:"available,omitempty"`
}

type Order struct {
	ID            string      `json:"id"`
	CustomerID    string      `json:"customer_id"`
	ReservationID string      `json:"reservation_id"`
	Items         []OrderItem `json:"items"`
	Status        OrderStatus `json:"status"`
	CouponCode    string      `json:"coupon_code,omitempty"`
	SubtotalKRW   int64       `json:"subtotal_krw"`
	DiscountKRW   int64       `json:"discount_krw"`
	// ShippingFeeKRW is the delivery charge in whole KRW, captured at order
	// creation so a later config change never re-prices a placed order.
	ShippingFeeKRW  int64           `json:"shipping_fee_krw"`
	TotalKRW        int64           `json:"total_krw"`
	RecipientName   string          `json:"recipient_name,omitempty"`
	RecipientPhone  string          `json:"recipient_phone,omitempty"`
	ShippingAddress ShippingAddress `json:"shipping_address,omitempty"`
	SourceAddressID string          `json:"source_address_id,omitempty"`
	PaymentID       string          `json:"payment_id,omitempty"`
	PaidAt          *time.Time      `json:"paid_at,omitempty"`
	PaymentDueAt    time.Time       `json:"payment_due_at"`
	ShippedBy       string          `json:"shipped_by,omitempty"`
	ShippedAt       *time.Time      `json:"shipped_at,omitempty"`
	Carrier         string          `json:"carrier,omitempty"`
	TrackingNumber  string          `json:"tracking_number,omitempty"`
	CarrierNote     string          `json:"carrier_note,omitempty"`
	// ConfirmedAt is when a manager accepted the paid order for fulfillment.
	// It is a timestamp on `paid` (and copied through ship), not a status.
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
	// CancelRequestedAt is set when the customer asks to cancel after
	// confirmation or once the order is in transit. Cleared on reject.
	CancelRequestedAt   *time.Time `json:"cancel_requested_at,omitempty"`
	CancelRequestReason string     `json:"cancel_request_reason,omitempty"`
	// Computed refund-policy flags — populated by ApplyRefundPolicy, not stored.
	ConfirmationDueAt        *time.Time `json:"confirmation_due_at,omitempty"`
	ConfirmationOverdue      bool       `json:"confirmation_overdue,omitempty"`
	CancelConfirmDueAt       *time.Time `json:"cancel_confirm_due_at,omitempty"`
	CancelConfirmOverdue     bool       `json:"cancel_confirm_overdue,omitempty"`
	ImmediateCancelAllowed   bool       `json:"immediate_cancel_allowed"`
	CancelRequestAllowed     bool       `json:"cancel_request_allowed"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
}

// NewOrder prices an order as subtotal - discount + shipping, all in whole KRW.
// shippingFeeKRW is passed in rather than read from config so the charge is
// snapshotted on the order: changing the configured fee must not alter what an
// already-placed order costs.
//
// The discount applies to goods only and is capped at the subtotal, so the total
// can never fall below the shipping fee — a 100%-off coupon still pays delivery.
func NewOrder(id, customerID, reservationID string, items []OrderItem, couponCode string, discountKRW, shippingFeeKRW int64, now time.Time) (*Order, error) {
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
		if (item.SKU == "" && item.SkuID == "") || item.Quantity <= 0 || item.UnitPriceKRW < 0 {
			return nil, ErrInvalidOrder
		}
		subtotal += int64(item.Quantity) * item.UnitPriceKRW
		copiedItems[i] = item
	}
	if len(copiedItems) == 0 {
		return nil, ErrInvalidOrder
	}
	if discountKRW < 0 || discountKRW > subtotal {
		return nil, ErrInvalidOrder
	}
	if shippingFeeKRW < 0 {
		return nil, ErrInvalidOrder
	}

	return &Order{
		ID:             id,
		CustomerID:     customerID,
		ReservationID:  reservationID,
		Items:          copiedItems,
		Status:         StatusPending,
		CouponCode:     strings.ToUpper(strings.TrimSpace(couponCode)),
		SubtotalKRW:    subtotal,
		DiscountKRW:    discountKRW,
		ShippingFeeKRW: shippingFeeKRW,
		TotalKRW:       subtotal - discountKRW + shippingFeeKRW,
		PaymentDueAt:   now.Add(DefaultPaymentTTL),
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func (o *Order) MarkPaid(paymentID string, amountKRW int64, now time.Time) error {
	if o.Status != StatusPending {
		return ErrInvalidTransition
	}
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return ErrInvalidOrder
	}
	if amountKRW != o.TotalKRW {
		return ErrPaymentAmountMismatch
	}
	o.Status = StatusPaid
	o.PaymentID = paymentID
	o.PaidAt = &now
	o.UpdatedAt = now
	return nil
}

func (o *Order) Ship(shippedBy string, tracking ShipmentTracking, now time.Time) error {
	if o.Status != StatusPaid {
		return ErrInvalidTransition
	}
	shippedBy = strings.TrimSpace(shippedBy)
	if shippedBy == "" {
		return ErrInvalidOrder
	}
	if tracking.Carrier == "" || tracking.TrackingNumber == "" {
		return fmt.Errorf("%w: carrier and tracking_number are required", ErrInvalidShipment)
	}
	if _, ok := ValidCarriers[tracking.Carrier]; !ok {
		return fmt.Errorf("%w: unknown carrier %q", ErrInvalidShipment, tracking.Carrier)
	}
	if tracking.Carrier == CarrierOther && strings.TrimSpace(tracking.CarrierNote) == "" {
		return fmt.Errorf("%w: carrier_note is required when carrier is other", ErrInvalidShipment)
	}
	o.Status = StatusInTransit
	o.ShippedBy = shippedBy
	o.ShippedAt = &now
	o.Carrier = tracking.Carrier
	o.TrackingNumber = tracking.TrackingNumber
	if tracking.Carrier == CarrierOther {
		o.CarrierNote = strings.TrimSpace(tracking.CarrierNote)
	} else {
		o.CarrierNote = ""
	}
	if o.ConfirmedAt == nil {
		o.ConfirmedAt = &now
	}
	o.UpdatedAt = now
	return nil
}

func (o *Order) Cancel(now time.Time) error {
	if o.Status != StatusPending && o.Status != StatusPaid && o.Status != StatusInTransit {
		return ErrInvalidTransition
	}
	o.Status = StatusCanceled
	o.UpdatedAt = now
	return nil
}

// IsManagerConfirmed reports whether a manager has accepted the order
// (explicit confirm, or ship which confirms as a side effect).
func (o *Order) IsManagerConfirmed() bool {
	if o == nil {
		return false
	}
	return o.ConfirmedAt != nil || o.Status == StatusInTransit || o.Status == StatusFulfilled
}

// AllowsImmediateCancel is true before manager confirmation: unpaid pending
// or paid-but-unconfirmed. Those cancels refund immediately.
func (o *Order) AllowsImmediateCancel() bool {
	if o == nil {
		return false
	}
	if o.Status == StatusPending {
		return true
	}
	return o.Status == StatusPaid && !o.IsManagerConfirmed()
}

// AllowsCancelRequest is true after confirmation or while in transit, when
// no cancel request is already pending.
func (o *Order) AllowsCancelRequest() bool {
	if o == nil || o.CancelRequestedAt != nil {
		return false
	}
	if o.Status == StatusInTransit {
		return true
	}
	return o.Status == StatusPaid && o.IsManagerConfirmed()
}

// Confirm records manager acceptance of a paid order. Idempotent when already confirmed.
func (o *Order) Confirm(now time.Time) error {
	if o.Status != StatusPaid {
		return ErrInvalidTransition
	}
	if o.ConfirmedAt != nil {
		return nil
	}
	o.ConfirmedAt = &now
	o.UpdatedAt = now
	return nil
}

// RequestCancel records a customer cancellation that needs manager approval.
// Idempotent when a request is already pending.
func (o *Order) RequestCancel(reason string, now time.Time) error {
	if o.CancelRequestedAt != nil {
		return nil
	}
	if o.Status != StatusInTransit && !(o.Status == StatusPaid && o.IsManagerConfirmed()) {
		return ErrInvalidTransition
	}
	o.CancelRequestedAt = &now
	o.CancelRequestReason = trimCancelReason(reason)
	o.UpdatedAt = now
	return nil
}

// RejectCancelRequest clears a pending customer cancel request.
func (o *Order) RejectCancelRequest(now time.Time) error {
	if o.CancelRequestedAt == nil {
		return nil
	}
	if o.Status != StatusPaid && o.Status != StatusInTransit {
		return ErrInvalidTransition
	}
	o.CancelRequestedAt = nil
	o.CancelRequestReason = ""
	o.UpdatedAt = now
	return nil
}

func (o *Order) ConfirmationDueAtTime() *time.Time {
	if o == nil || o.PaidAt == nil || o.ConfirmedAt != nil || o.Status != StatusPaid {
		return nil
	}
	due := o.PaidAt.Add(ManagerConfirmationWindow)
	return &due
}

func (o *Order) CancelConfirmDueAtTime() *time.Time {
	if o == nil || o.CancelRequestedAt == nil {
		return nil
	}
	if o.Status != StatusPaid && o.Status != StatusInTransit {
		return nil
	}
	due := o.CancelRequestedAt.Add(ManagerConfirmationWindow)
	return &due
}

func (o *Order) ConfirmationOverdueAt(now time.Time) bool {
	due := o.ConfirmationDueAtTime()
	return due != nil && !now.Before(*due)
}

func (o *Order) CancelConfirmOverdueAt(now time.Time) bool {
	due := o.CancelConfirmDueAtTime()
	return due != nil && !now.Before(*due)
}

// ApplyRefundPolicy fills computed JSON fields for API responses.
func (o *Order) ApplyRefundPolicy(now time.Time) {
	if o == nil {
		return
	}
	o.ConfirmationDueAt = o.ConfirmationDueAtTime()
	o.ConfirmationOverdue = o.ConfirmationOverdueAt(now)
	o.CancelConfirmDueAt = o.CancelConfirmDueAtTime()
	o.CancelConfirmOverdue = o.CancelConfirmOverdueAt(now)
	o.ImmediateCancelAllowed = o.AllowsImmediateCancel()
	o.CancelRequestAllowed = o.AllowsCancelRequest()
}

func trimCancelReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if len(reason) <= MaxCancelRequestReasonLen {
		return reason
	}
	return reason[:MaxCancelRequestReasonLen]
}

// ReinstateForLatePayment moves an auto-canceled pending order back to pending
// with a fresh stock reservation so a payment that completes after expiry can
// still be applied.
func (o *Order) ReinstateForLatePayment(reservationID string, now time.Time) error {
	if o.Status != StatusCanceled {
		return ErrInvalidTransition
	}
	reservationID = strings.TrimSpace(reservationID)
	if reservationID == "" {
		return ErrInvalidOrder
	}
	o.Status = StatusPending
	o.ReservationID = reservationID
	o.PaymentDueAt = now.Add(DefaultPaymentTTL)
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
