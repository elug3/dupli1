package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

type PromotionService struct {
	store  ports.PromotionStore
	ledger ports.PromotionRedemptionStore
	now    func() time.Time
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

	// A single-user code needs an entitlement, which is Phase 3. Until then it
	// is refused rather than silently treated as a public code.
	if promotion.EffectiveScope() == domain.ScopeSingleUser {
		return domain.EvaluationResult{Reason: domain.ReasonLoginRequired}
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
