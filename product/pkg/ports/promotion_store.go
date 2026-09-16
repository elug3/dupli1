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
