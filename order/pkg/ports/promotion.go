package ports

import (
	"context"
	"errors"
)

var (
	ErrPromotionInvalid     = errors.New("invalid promotion code")
	ErrPromotionUnavailable = errors.New("promotion service unavailable")
	// ErrPromotionNotEligible is a code that exists but this cart has not
	// earned. The reason travels with it so the storefront can say why.
	ErrPromotionNotEligible = errors.New("promotion not eligible")
)

// PromotionLine is one priced cart line as the evaluator needs to see it.
// Order builds these after server-side pricing, so UnitPriceWon is resolved,
// never a number the client sent.
//
// Identity, quantity and price are all order has and all it sends. A
// condition on a line's category, brand, parent or sale state is resolved by
// product from its own catalog, keyed on these identifiers — those fields
// deliberately do not travel here, both because order would have to carry
// catalog columns it has no other use for and because a discount must not
// depend on what its caller claims a cart contains.
type PromotionLine struct {
	SkuID        string `json:"sku_id"`
	SKU          string `json:"sku"`
	Quantity     int    `json:"quantity"`
	UnitPriceWon int64  `json:"unit_price_won"`
}

// PromotionContext is the checkout a code is judged against.
type PromotionContext struct {
	CustomerID     string          `json:"customer_id"`
	ShippingFeeWon int64           `json:"shipping_fee_won"`
	Lines          []PromotionLine `json:"lines"`
}

// PromotionEvaluation is product's verdict.
//
// A refusal is not a transport error: OK is false and Reason says why, so the
// storefront can render the right copy instead of a generic failure.
type PromotionEvaluation struct {
	OK                  bool     `json:"ok"`
	Code                string   `json:"code,omitempty"`
	DiscountWon         int64    `json:"discount_won"`
	ShippingDiscountWon int64    `json:"shipping_discount_won"`
	EligibleSkuIDs      []string `json:"eligible_sku_ids,omitempty"`
	EligibleSubtotalWon int64    `json:"eligible_subtotal_won"`
	Reason              string   `json:"reason,omitempty"`
	SubReason           string   `json:"sub_reason,omitempty"`
}

// PromotionClient talks to product, which owns promotional code definitions
// and the usage ledger.
type PromotionClient interface {
	// Evaluate judges a code against a cart and returns the discount it earns.
	Evaluate(ctx context.Context, code string, promoCtx PromotionContext) (*PromotionEvaluation, error)
	// Reserve re-evaluates and records a pending use against an order. It is
	// idempotent per order, so a retried checkout complete does not burn a
	// second use.
	Reserve(ctx context.Context, code, orderID string, promoCtx PromotionContext) (*PromotionEvaluation, error)
	// Consume marks an order's reservation paid.
	Consume(ctx context.Context, orderID string) error
	// Release hands the use back, for a cancel before shipment.
	Release(ctx context.Context, orderID string) error
}
