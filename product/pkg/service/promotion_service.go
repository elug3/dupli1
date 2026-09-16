package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

type PromotionService struct {
	store        ports.PromotionStore
	ledger       ports.PromotionRedemptionStore
	entitlements ports.PromotionEntitlementStore
	now          func() time.Time
}

func NewPromotionService(store ports.PromotionStore) *PromotionService {
	return &PromotionService{store: store, now: func() time.Time { return time.Now().UTC() }}
}

// WithLedger attaches the usage ledger. Without it a code still evaluates, but
// nothing enforces once-per-customer — which is how every code behaved before
// Phase 2, and is why bootstrap always wires one.
func (s *PromotionService) WithLedger(ledger ports.PromotionRedemptionStore) *PromotionService {
	s.ledger = ledger
	return s
}

// WithEntitlements attaches the single-user access store. Without it a
// single_user code cannot be used at all, because nothing can say who holds it.
func (s *PromotionService) WithEntitlements(store ports.PromotionEntitlementStore) *PromotionService {
	s.entitlements = store
	return s
}

// WithClock is for tests that need to sit on an expiry boundary.
func (s *PromotionService) WithClock(now func() time.Time) *PromotionService {
	if now != nil {
		s.now = now
	}
	return s
}

func (s *PromotionService) List(ctx context.Context) []domain.Promotion {
	promotions, err := s.store.List(ctx)
	if err != nil {
		return nil
	}
	return promotions
}

func (s *PromotionService) Create(ctx context.Context, p domain.Promotion) (*domain.Promotion, error) {
	p.Code = domain.NormalizedCode(p.Code)
	if p.Code == "" {
		return nil, ports.Invalid("code is required")
	}
	if p.Scope == "" {
		p.Scope = domain.ScopeGlobal
	}
	if err := validateDefinition(p); err != nil {
		return nil, err
	}
	if err := s.store.Create(ctx, p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *PromotionService) Update(ctx context.Context, code string, patch ports.PromotionPatch) (*domain.Promotion, error) {
	// Validate the result of the patch, not the patch itself: a partial update
	// that would leave the definition invalid must be refused before it is
	// written, not discovered later at a customer's checkout.
	current, err := s.store.Get(ctx, code)
	if err != nil {
		return nil, err
	}
	preview := *current
	applyPatchForValidation(&preview, patch)
	if err := validateDefinition(preview); err != nil {
		return nil, err
	}
	return s.store.Update(ctx, code, patch)
}

func (s *PromotionService) Delete(ctx context.Context, code string) error {
	return s.store.Delete(ctx, code)
}

// Redeem is the public lookup behind POST /promotions/redeem. It answers only
// "is this a live code", so it deliberately cannot see a cart: eligibility and
// the discount amount come from Evaluate.
func (s *PromotionService) Redeem(ctx context.Context, code string) (*domain.Promotion, bool) {
	promotion, ok := s.store.GetActive(ctx, code)
	if !ok {
		return nil, false
	}
	if promotion.IsExpired(s.now()) || promotion.IsExhausted() {
		return nil, false
	}
	return promotion, true
}

// Evaluate judges a code against a specific checkout and returns the discount.
//
// Order calls this both when a customer applies a code and again at checkout
// complete, so edits to the cart between the two cannot carry a discount the
// cart no longer earns.
func (s *PromotionService) Evaluate(ctx context.Context, code string, evalCtx domain.EvaluationContext) domain.EvaluationResult {
	return s.evaluate(ctx, code, evalCtx, true)
}

// evaluate is Evaluate with control over the per-customer check.
//
// Reserve passes checkCustomerLimit=false and lets the ledger decide instead.
// The ledger holds the limit and the order's own reservation in one
// transaction, so it can tell "this customer already used the code" from
// "this is a retry of the same order" — a check here cannot, and counting the
// order's own reservation against it made a retried complete look like a
// second use.
func (s *PromotionService) evaluate(ctx context.Context, code string, evalCtx domain.EvaluationContext, checkCustomerLimit bool) domain.EvaluationResult {
	promotion, err := s.store.Get(ctx, domain.NormalizedCode(code))
	if err != nil {
		// An unknown code and a soft-deleted one are reported identically, so
		// that probing cannot enumerate live campaigns.
		return domain.EvaluationResult{Reason: domain.ReasonInvalidCode}
	}
	if evalCtx.Now.IsZero() {
		evalCtx.Now = s.now()
	}

	// A single-user code is unusable without an entitlement.
	if promotion.EffectiveScope() == domain.ScopeSingleUser {
		if evalCtx.CustomerID == "" {
			return domain.EvaluationResult{Reason: domain.ReasonLoginRequired}
		}
		if s.entitlements == nil {
			return domain.EvaluationResult{Reason: domain.ReasonInvalidCode}
		}
		entitlement, err := s.entitlements.Find(ctx, promotion.Code, evalCtx.CustomerID)
		if err != nil {
			// Not holding the entitlement is reported the same as the code not
			// existing, so guessing a campaign's code tells you nothing.
			return domain.EvaluationResult{Reason: domain.ReasonInvalidCode}
		}
		if entitlement.RevokedAt != nil {
			return domain.EvaluationResult{Reason: domain.ReasonInvalidCode}
		}
		if !entitlement.Usable(evalCtx.Now) {
			return domain.EvaluationResult{Reason: domain.ReasonExpired}
		}
	}

	result := promotion.Evaluate(evalCtx)
	if !result.OK {
		return result
	}

	// Once-per-customer needs stored state, so it is checked here rather than
	// in the pure evaluator.
	if checkCustomerLimit && s.ledger != nil && evalCtx.CustomerID != "" {
		used, err := s.ledger.ActiveCountForCustomer(ctx, promotion.Code, evalCtx.CustomerID)
		if err != nil {
			return domain.EvaluationResult{Reason: domain.ReasonNotEligible, SubReason: domain.SubReasonCondition}
		}
		if used >= promotion.EffectiveMaxPerCustomer() {
			return domain.EvaluationResult{Reason: domain.ReasonAlreadyUsed}
		}
	}
	return result
}

// Reserve records a pending use at checkout complete. It re-evaluates first so
// a reservation can never be created for a cart that does not earn it.
func (s *PromotionService) Reserve(ctx context.Context, code, orderID string, evalCtx domain.EvaluationContext) (*domain.Redemption, domain.EvaluationResult, error) {
	result := s.evaluate(ctx, code, evalCtx, false)
	if !result.OK {
		return nil, result, nil
	}
	if s.ledger == nil {
		return nil, result, nil
	}
	promotion, err := s.store.Get(ctx, domain.NormalizedCode(code))
	if err != nil {
		return nil, domain.EvaluationResult{Reason: domain.ReasonInvalidCode}, nil
	}

	row, err := s.ledger.Reserve(ctx, ports.ReserveRedemptionInput{
		Code:                promotion.Code,
		OrderID:             orderID,
		CustomerID:          evalCtx.CustomerID,
		DiscountWon:         result.DiscountWon,
		ShippingDiscountWon: result.ShippingDiscountWon,
		OrderSubtotalWon:    evalCtx.SubtotalWon(),
		EligibleSubtotalWon: result.EligibleSubtotalWon,
		AppliedBenefit:      result.AppliedBenefit,
	}, promotion.EffectiveMaxPerCustomer())
	if err != nil {
		if errors.Is(err, ports.ErrConflict) {
			return nil, domain.EvaluationResult{Reason: domain.ReasonAlreadyUsed}, nil
		}
		return nil, domain.EvaluationResult{}, err
	}
	return row, result, nil
}

// Consume marks an order's reservation paid.
func (s *PromotionService) Consume(ctx context.Context, orderID string) error {
	if s.ledger == nil {
		return nil
	}
	return s.ledger.Consume(ctx, orderID, s.now())
}

// Release hands a use back, for a cancel before shipment. Callers decide
// whether a given cancel is releasable — an order cancelled after it shipped
// keeps its redemption consumed.
func (s *PromotionService) Release(ctx context.Context, orderID string) error {
	if s.ledger == nil {
		return nil
	}
	return s.ledger.Release(ctx, orderID, s.now())
}

func validateDefinition(p domain.Promotion) error {
	switch p.EffectiveScope() {
	case domain.ScopeGlobal, domain.ScopeSingleUser:
	default:
		return ports.Invalid(fmt.Sprintf("scope %q is not one of global, single_user", p.Scope))
	}
	if err := p.Conditions.Validate(); err != nil {
		return ports.Invalid(err.Error())
	}
	// A definition may carry no benefit document only while the legacy
	// percentage column is still meaningful; otherwise it must be explicit.
	if p.Benefit.IsZero() {
		if p.Discount <= 0 || p.Discount >= 1 {
			return ports.Invalid("a benefit is required (discount_fraction must be between 0 and 1 exclusive)")
		}
		return nil
	}
	if err := p.Benefit.Validate(); err != nil {
		return ports.Invalid(err.Error())
	}
	if p.MaxRedemptions != nil && *p.MaxRedemptions <= 0 {
		return ports.Invalid("max_redemptions must be greater than 0 when set")
	}
	if p.MaxPerCustomer < 0 {
		return ports.Invalid("max_per_customer cannot be negative")
	}
	return nil
}

// applyPatchForValidation mirrors the stores' folding so Update can validate
// the post-patch definition before writing it.
func applyPatchForValidation(p *domain.Promotion, patch ports.PromotionPatch) {
	if patch.Scope != nil {
		p.Scope = *patch.Scope
	}
	if patch.Conditions != nil {
		p.Conditions = *patch.Conditions
	}
	if patch.Benefit != nil {
		p.Benefit = *patch.Benefit
	}
	if patch.MaxPerCustomer != nil {
		p.MaxPerCustomer = *patch.MaxPerCustomer
	}
	if patch.ClearMaxRedemptions {
		p.MaxRedemptions = nil
	} else if patch.MaxRedemptions != nil {
		p.MaxRedemptions = patch.MaxRedemptions
	}
	if patch.Discount != nil {
		p.Discount = *patch.Discount
	}
}

// ── Entitlements ─────────────────────────────────────────────────────────────

// Issue grants one account the right to use a single-user code.
//
// Idempotent on (code, customer_id, trigger_key): the registration subscriber
// and the backfill both pass a stable key, so a redelivered event or a re-run
// mints nothing. The entitlement's expiry is computed from the definition at
// issue time, so an account issued today gets the same window as one issued at
// launch.
func (s *PromotionService) Issue(ctx context.Context, code, customerID, source, triggerKey, issuedBy string) (*domain.CustomerPromotion, error) {
	if s.entitlements == nil {
		return nil, ports.Invalid("entitlements are not configured")
	}
	customerID = strings.TrimSpace(customerID)
	if customerID == "" {
		return nil, ports.Invalid("customer_id is required")
	}
	promotion, err := s.store.Get(ctx, domain.NormalizedCode(code))
	if err != nil {
		return nil, err
	}
	if promotion.EffectiveScope() != domain.ScopeSingleUser {
		return nil, ports.Invalid("only single_user promotional codes are issued to an account")
	}
	return s.entitlements.Issue(ctx, ports.IssueEntitlementInput{
		Code:       promotion.Code,
		CustomerID: customerID,
		Source:     source,
		TriggerKey: triggerKey,
		IssuedBy:   issuedBy,
		ExpiresAt:  promotion.EntitlementExpiry(s.now()),
	})
}

// Revoke withdraws an entitlement. It never rewrites an order that already
// used the code.
func (s *PromotionService) Revoke(ctx context.Context, id string) error {
	if s.entitlements == nil {
		return ports.Invalid("entitlements are not configured")
	}
	return s.entitlements.Revoke(ctx, id, s.now())
}

// WalletEntry is one entitlement as the storefront shows it: the definition it
// grants, and whether it can be used against the cart the customer has now.
type WalletEntry struct {
	Entitlement domain.CustomerPromotion `json:"entitlement"`
	Promotion   *domain.Promotion        `json:"promotion,omitempty"`
	Eligible    bool                     `json:"eligible"`
	DiscountWon int64                    `json:"discount_won"`
	Reason      domain.Reason            `json:"reason,omitempty"`
	SubReason   string                   `json:"sub_reason,omitempty"`
}

// Wallet lists a customer's entitlements, each judged against the cart they
// are looking at.
//
// Ineligible entries are returned rather than hidden, with the reason, so the
// storefront can say "spend 100,000원" instead of silently dropping a code the
// customer knows they have.
func (s *PromotionService) Wallet(ctx context.Context, customerID string, evalCtx domain.EvaluationContext) ([]WalletEntry, error) {
	if s.entitlements == nil {
		return nil, nil
	}
	rows, err := s.entitlements.ListForCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	evalCtx.CustomerID = customerID
	if evalCtx.Now.IsZero() {
		evalCtx.Now = s.now()
	}

	out := make([]WalletEntry, 0, len(rows))
	for _, row := range rows {
		entry := WalletEntry{Entitlement: row}
		promotion, err := s.store.Get(ctx, row.Code)
		if err == nil {
			entry.Promotion = promotion
		}
		result := s.evaluate(ctx, row.Code, evalCtx, true)
		entry.Eligible = result.OK
		entry.DiscountWon = result.DiscountWon
		entry.Reason = result.Reason
		entry.SubReason = result.SubReason
		out = append(out, entry)
	}
	return out, nil
}
