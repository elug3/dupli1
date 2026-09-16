package domain

import "time"

// RedemptionStatus tracks one use of a promotional code against one order.
//
// The lifecycle mirrors the stock rule: nothing is really spent until the
// money is, and nothing is given back once the goods have shipped.
//
//	reserved  at checkout complete
//	consumed  on payment.succeeded
//	released  on cancel from pending or paid — but never from in_transit
//
// Reports count consumed rows only; released rows are kept for audit.
type RedemptionStatus string

const (
	RedemptionReserved RedemptionStatus = "reserved"
	RedemptionConsumed RedemptionStatus = "consumed"
	RedemptionReleased RedemptionStatus = "released"
)

// Redemption is one ledger row.
type Redemption struct {
	ID         string           `json:"id"`
	Code       string           `json:"code"`
	OrderID    string           `json:"order_id"`
	CustomerID string           `json:"customer_id"`
	Status     RedemptionStatus `json:"status"`

	DiscountWon         int64 `json:"discount_won"`
	ShippingDiscountWon int64 `json:"shipping_discount_won"`
	OrderSubtotalWon    int64 `json:"order_subtotal_won"`
	EligibleSubtotalWon int64 `json:"eligible_subtotal_won"`

	// AppliedBenefit is the benefit as it stood when this redemption was
	// reserved. Editing a live definition is allowed and never retroactive, so
	// reports read this snapshot rather than the current definition.
	AppliedBenefit *Benefit `json:"applied_benefit,omitempty"`

	CreatedAt  time.Time  `json:"created_at"`
	PaidAt     *time.Time `json:"paid_at,omitempty"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
}

// CountsAgainstCustomer reports whether this row occupies the customer's one
// permitted use. A released row does not: the order it belonged to was
// canceled before shipment, so the use is handed back.
func (r Redemption) CountsAgainstCustomer() bool {
	return r.Status == RedemptionReserved || r.Status == RedemptionConsumed
}
