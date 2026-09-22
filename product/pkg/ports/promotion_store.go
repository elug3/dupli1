package ports

import (
	"context"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
)

// PromotionPatch is a partial update of a promotional code definition.
//
// A struct rather than a positional argument list: the pre-Phase-2 signature
// already took four pointers, and Phase 2 adds eight more fields. Omitted
// fields keep their current value. Where a field is nullable, a separate
// Clear* flag distinguishes "leave it alone" from "unset it", which a lone
// pointer cannot express.
type PromotionPatch struct {
	Description    *string
	Active         *bool
	Terms          *string
	Scope          *domain.Scope
	Conditions     *domain.Conditions
	Benefit        *domain.Benefit
	MaxPerCustomer *int
	// EntitlementTTLDays is how long an issued single-user entitlement lasts.
	EntitlementTTLDays *int

	ExpiresAt      *time.Time
	ClearExpiresAt bool

	MaxRedemptions      *int
	ClearMaxRedemptions bool

	// Discount and Expires are the pre-Phase-2 columns, still writable during
	// the rename/upgrade window. See docs/product-promotion-rename.md.
	Discount *float64
	Expires  *string
}

type PromotionStore interface {
	List(ctx context.Context) ([]domain.Promotion, error)
	Create(ctx context.Context, p domain.Promotion) error
	Update(ctx context.Context, code string, patch PromotionPatch) (*domain.Promotion, error)
	Delete(ctx context.Context, code string) error
	// Get returns a definition whatever its state, so callers can tell an
	// expired or exhausted code from one that does not exist.
	Get(ctx context.Context, code string) (*domain.Promotion, error)
	GetActive(ctx context.Context, code string) (*domain.Promotion, bool)
}

// ReserveRedemptionInput is one pending use of a code against an order.
type ReserveRedemptionInput struct {
	Code                string
	OrderID             string
	CustomerID          string
	DiscountWon         int64
	ShippingDiscountWon int64
	OrderSubtotalWon    int64
	EligibleSubtotalWon int64
	AppliedBenefit      *domain.Benefit
}

// PromotionRedemptionStore is the usage ledger. It is what makes a code
// finite: without it a definition can be applied without limit, which is how
// every code behaved before Phase 2.
type PromotionRedemptionStore interface {
	// Reserve records a pending use. It fails with ErrConflict when the
	// customer has already used the code up to its per-customer limit, which
	// is the once-per-customer guarantee.
	Reserve(ctx context.Context, in ReserveRedemptionInput, maxPerCustomer int) (*domain.Redemption, error)
	// Consume marks an order's reservation paid.
	Consume(ctx context.Context, orderID string, at time.Time) error
	// Release hands the use back, for a cancel before shipment.
	Release(ctx context.Context, orderID string, at time.Time) error
	// ActiveCountForCustomer counts reserved plus consumed rows.
	ActiveCountForCustomer(ctx context.Context, code, customerID string) (int, error)
	// CountsByCode returns reserved+consumed and consumed-only totals, for the
	// campaign cap and for reporting respectively.
	CountsByCode(ctx context.Context, code string) (active int, consumed int, err error)
}

// IssueEntitlementInput grants one account the right to use a single-user code.
type IssueEntitlementInput struct {
	Code       string
	CustomerID string
	Source     string
	TriggerKey string
	IssuedBy   string
	ExpiresAt  *time.Time
}

// PromotionEntitlementStore holds who may use which single-user code.
//
// It answers access only. Whether a code has been spent is the redemption
// ledger's job, so there is one writer for that fact rather than two that can
// drift.
type PromotionEntitlementStore interface {
	// Issue grants an entitlement. It is idempotent on
	// (code, customer_id, trigger_key): a redelivered registration event
	// returns the existing row rather than minting a second.
	Issue(ctx context.Context, in IssueEntitlementInput) (*domain.CustomerPromotion, error)
	// Find returns this customer's entitlement for a code, if any.
	Find(ctx context.Context, code, customerID string) (*domain.CustomerPromotion, error)
	// ListForCustomer returns every entitlement an account holds, newest first.
	ListForCustomer(ctx context.Context, customerID string) ([]domain.CustomerPromotion, error)
	// Revoke withdraws an entitlement. It never rewrites an order that already
	// used it.
	Revoke(ctx context.Context, id string, at time.Time) error
}

// LineRef identifies one cart line to the catalog. Callers send whichever
// identifier they hold; sku_id is canonical, sku is the human string.
type LineRef struct {
	SkuID string
	SKU   string
}

// LineCatalog is what the catalog knows about a cart line. Everything here is
// read from the seller's own data rather than taken from the caller, so a
// client cannot claim a brand or a category to earn a discount.
type LineCatalog struct {
	Found     bool
	SkuID     string
	SKU       string
	ProductID string
	Category  string
	BrandCode string
	// OnSale is the parent's official price standing above its selling price.
	OnSale bool
}

// PromotionCatalog resolves the catalog attributes a condition can address.
//
// Conditions can gate on a line's category, brand, parent or sale state, and
// none of those travel with a checkout: order builds its evaluation lines from
// priced order items, which carry identity, quantity and price and nothing
// else. Rather than plumb catalog columns through checkout — where a client
// could also supply them — the evaluator reads them here, from the service
// that owns the catalog.
type PromotionCatalog interface {
	// LineAttributes returns one entry per ref, in the same order. A line the
	// catalog cannot resolve comes back with Found false rather than an error,
	// so one unknown SKU cannot fail a whole evaluation.
	LineAttributes(ctx context.Context, refs []LineRef) ([]LineCatalog, error)
}
