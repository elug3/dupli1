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
// payment and manager approval of a post-confirm / in-transit / delivered
// cancel request.
const ManagerConfirmationWindow = 2 * time.Hour

// DeliveryAutoFulfillWindow is how long a customer has to confirm receipt (or
// report a problem) after delivery before the order is considered resolved
// and auto-fulfilled.
const DeliveryAutoFulfillWindow = 14 * 24 * time.Hour

// MaxCancelRequestReasonLen caps the optional customer cancel-request note.
const MaxCancelRequestReasonLen = 500

// MaxDisputeReasonLen caps the optional customer "not received" note.
const MaxDisputeReasonLen = 500

// ReasonVariantNotFound is returned when a line cannot be resolved to an
// active, sellable product variant.
const ReasonVariantNotFound = "variant_not_found"

type OrderStatus string

const (
	StatusPending OrderStatus = "pending"
	StatusPaid    OrderStatus = "paid"
	// StatusConfirmed is a manager's acceptance of a paid order for
	// fulfillment — the gate before shipping.
	StatusConfirmed OrderStatus = "confirmed"
	// StatusInTransit is the shipping stage: a manager has handed the order
	// to a carrier.
	StatusInTransit OrderStatus = "in_transit"
	// StatusDelivered is the carrier (or a manager) marking the parcel as
	// handed to the customer. Not final: the customer still confirms receipt,
	// disputes it, or the 14-day auto-fulfill window closes it out.
	StatusDelivered OrderStatus = "delivered"
	StatusFulfilled OrderStatus = "fulfilled"
	// StatusDisputed is a customer's "I did not receive this" on a delivered
	// order, pending manager review.
	StatusDisputed OrderStatus = "disputed"
	StatusCanceled OrderStatus = "canceled"
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
	UnitPriceWon int64  `json:"unit_price_won"` // whole KRW won
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
	SubtotalWon   int64       `json:"subtotal_won"`
	DiscountWon   int64       `json:"discount_won"`
	// ShippingFeeWon is the delivery charge in whole KRW, captured at order
	// creation so a later config change never re-prices a placed order.
	ShippingFeeWon  int64           `json:"shipping_fee_won"`
	TotalWon        int64           `json:"total_won"`
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
	// ConfirmedAt is when a manager accepted the paid order for fulfillment
	// (transitions paid → confirmed).
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
	// DeliveredAt/DeliveredBy record the carrier or manager marking the
	// shipment delivered (transitions in_transit → delivered).
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
	DeliveredBy string     `json:"delivered_by,omitempty"`
	// ReceiptConfirmedAt is when the customer confirmed they received the
	// order (transitions delivered → fulfilled). Nil when fulfillment came
	// from the 14-day auto-fulfill sweep or a manager override instead.
	ReceiptConfirmedAt *time.Time `json:"receipt_confirmed_at,omitempty"`
	// DisputedAt/DisputeReason record a customer's "I did not receive this"
	// on a delivered order (transitions delivered → disputed).
	DisputedAt    *time.Time `json:"disputed_at,omitempty"`
	DisputeReason string     `json:"dispute_reason,omitempty"`
	// CancelRequestedAt is set when the customer asks to cancel after
	// confirmation, in transit, or once delivered. Cleared on reject.
	CancelRequestedAt   *time.Time `json:"cancel_requested_at,omitempty"`
	CancelRequestReason string     `json:"cancel_request_reason,omitempty"`
	// Computed refund/delivery-policy flags — populated by ApplyRefundPolicy,
	// not stored.
	ConfirmationDueAt      *time.Time `json:"confirmation_due_at,omitempty"`
	ConfirmationOverdue    bool       `json:"confirmation_overdue,omitempty"`
	CancelConfirmDueAt     *time.Time `json:"cancel_confirm_due_at,omitempty"`
	CancelConfirmOverdue   bool       `json:"cancel_confirm_overdue,omitempty"`
	ImmediateCancelAllowed bool       `json:"immediate_cancel_allowed"`
	CancelRequestAllowed   bool       `json:"cancel_request_allowed"`
	// AutoFulfillDueAt is when a delivered order with no customer response or
	// dispute is auto-fulfilled.
	AutoFulfillDueAt *time.Time `json:"auto_fulfill_due_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
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
		if (item.SKU == "" && item.SkuID == "") || item.Quantity <= 0 || item.UnitPriceWon < 0 {
			return nil, ErrInvalidOrder
		}
		subtotal += int64(item.Quantity) * item.UnitPriceWon
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
		SubtotalWon:    subtotal,
		DiscountWon:    discountKRW,
		ShippingFeeWon: shippingFeeKRW,
		TotalWon:       subtotal - discountKRW + shippingFeeKRW,
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
	if amountKRW != o.TotalWon {
		return ErrPaymentAmountMismatch
	}
	o.Status = StatusPaid
	o.PaymentID = paymentID
	o.PaidAt = &now
	o.UpdatedAt = now
	return nil
}

func (o *Order) Ship(shippedBy string, tracking ShipmentTracking, now time.Time) error {
	if o.Status != StatusConfirmed {
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
	o.UpdatedAt = now
	return nil
}

// Deliver records the carrier (or a manager) handing the parcel to the
// customer. Not final: the customer still confirms receipt, disputes it, or
// the 14-day auto-fulfill window closes it out.
func (o *Order) Deliver(deliveredBy string, now time.Time) error {
	if o.Status != StatusInTransit {
		return ErrInvalidTransition
	}
	o.Status = StatusDelivered
	o.DeliveredAt = &now
	o.DeliveredBy = strings.TrimSpace(deliveredBy)
	o.UpdatedAt = now
	return nil
}

// ConfirmReceipt records the customer confirming they received the order.
func (o *Order) ConfirmReceipt(now time.Time) error {
	if o.Status != StatusDelivered {
		return ErrInvalidTransition
	}
	o.Status = StatusFulfilled
	o.ReceiptConfirmedAt = &now
	o.UpdatedAt = now
	return nil
}

// ReportNotReceived opens a manager-reviewed dispute on a delivered order the
// customer says never arrived.
//
// Clears any still-pending cancel request: a customer can request a cancel
// and then, before a manager answers it, also report non-receipt on the same
// delivered order. The dispute supersedes it — AllowsCancelRequest is false
// once disputed, so a stale request would otherwise show a cancel-request
// banner with no manager-response deadline attached.
func (o *Order) ReportNotReceived(reason string, now time.Time) error {
	if o.Status != StatusDelivered {
		return ErrInvalidTransition
	}
	o.Status = StatusDisputed
	o.DisputedAt = &now
	o.DisputeReason = trimReason(reason, MaxDisputeReasonLen)
	o.CancelRequestedAt = nil
	o.CancelRequestReason = ""
	o.UpdatedAt = now
	return nil
}

// ResolveDisputeFulfilled closes a dispute in the delivery's favor — a
// manager found the parcel was in fact delivered (proof of delivery, the
// customer found it, etc.) — without a refund.
func (o *Order) ResolveDisputeFulfilled(now time.Time) error {
	if o.Status != StatusDisputed {
		return ErrInvalidTransition
	}
	o.Status = StatusFulfilled
	o.UpdatedAt = now
	return nil
}

func (o *Order) Cancel(now time.Time) error {
	if !o.Cancelable() {
		return ErrInvalidTransition
	}
	o.Status = StatusCanceled
	o.UpdatedAt = now
	return nil
}

// Cancelable reports whether Cancel is a valid transition from the order's
// current status — every status except the two final ones.
func (o *Order) Cancelable() bool {
	switch o.Status {
	case StatusFulfilled, StatusCanceled:
		return false
	default:
		return true
	}
}

// StockCommitted reports whether inventory was already committed for this
// order (at ship time), so a cancel from here on refunds but does not
// automatically restock.
func (o *Order) StockCommitted() bool {
	switch o.Status {
	case StatusInTransit, StatusDelivered, StatusDisputed:
		return true
	default:
		return false
	}
}

// AllowsImmediateCancel is true before manager confirmation: unpaid pending
// or paid-but-unconfirmed. Those cancels refund immediately.
func (o *Order) AllowsImmediateCancel() bool {
	if o == nil {
		return false
	}
	return o.Status == StatusPending || o.Status == StatusPaid
}

// AllowsCancelRequest is true once a manager has confirmed the order and up
// through delivery, when no cancel request is already pending. Once a
// dispute is open the dispute is the mechanism instead.
func (o *Order) AllowsCancelRequest() bool {
	if o == nil || o.CancelRequestedAt != nil {
		return false
	}
	switch o.Status {
	case StatusConfirmed, StatusInTransit, StatusDelivered:
		return true
	default:
		return false
	}
}

// Confirm records manager acceptance of a paid order. Idempotent when already confirmed.
func (o *Order) Confirm(now time.Time) error {
	if o.Status == StatusConfirmed {
		return nil
	}
	if o.Status != StatusPaid {
		return ErrInvalidTransition
	}
	o.Status = StatusConfirmed
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
	if !o.AllowsCancelRequest() {
		return ErrInvalidTransition
	}
	o.CancelRequestedAt = &now
	o.CancelRequestReason = trimReason(reason, MaxCancelRequestReasonLen)
	o.UpdatedAt = now
	return nil
}

// RejectCancelRequest clears a pending customer cancel request.
func (o *Order) RejectCancelRequest(now time.Time) error {
	if o.CancelRequestedAt == nil {
		return nil
	}
	switch o.Status {
	case StatusConfirmed, StatusInTransit, StatusDelivered:
		// continue
	default:
		return ErrInvalidTransition
	}
	o.CancelRequestedAt = nil
	o.CancelRequestReason = ""
	o.UpdatedAt = now
	return nil
}

func (o *Order) ConfirmationDueAtTime() *time.Time {
	if o == nil || o.PaidAt == nil || o.Status != StatusPaid {
		return nil
	}
	due := o.PaidAt.Add(ManagerConfirmationWindow)
	return &due
}

func (o *Order) CancelConfirmDueAtTime() *time.Time {
	if o == nil || o.CancelRequestedAt == nil {
		return nil
	}
	switch o.Status {
	case StatusConfirmed, StatusInTransit, StatusDelivered:
		// continue
	default:
		return nil
	}
	due := o.CancelRequestedAt.Add(ManagerConfirmationWindow)
	return &due
}

// AutoFulfillDueAtTime is when a delivered order with no customer response
// (and no open dispute) is auto-fulfilled.
func (o *Order) AutoFulfillDueAtTime() *time.Time {
	if o == nil || o.DeliveredAt == nil || o.Status != StatusDelivered {
		return nil
	}
	due := o.DeliveredAt.Add(DeliveryAutoFulfillWindow)
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

// AutoFulfillDueAt reports whether a delivered order's 14-day response
// window has closed with no customer action.
func (o *Order) AutoFulfillOverdueAt(now time.Time) bool {
	due := o.AutoFulfillDueAtTime()
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
	o.AutoFulfillDueAt = o.AutoFulfillDueAtTime()
}

func trimReason(reason string, maxLen int) string {
	reason = strings.TrimSpace(reason)
	if len(reason) <= maxLen {
		return reason
	}
	return reason[:maxLen]
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

// Fulfill is the manager override / 14-day auto-fulfill path: closes out a
// delivered (or disputed) order without going through the customer's own
// ConfirmReceipt.
func (o *Order) Fulfill(now time.Time) error {
	if o.Status != StatusDelivered && o.Status != StatusDisputed {
		return ErrInvalidTransition
	}
	o.Status = StatusFulfilled
	o.UpdatedAt = now
	return nil
}

func (o *Order) IsPaymentExpired(now time.Time) bool {
	return o.Status == StatusPending && now.After(o.PaymentDueAt)
}
