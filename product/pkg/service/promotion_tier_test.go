package service_test

import (
	"context"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
)

// tierPromotion is a customer tier as a manager registers it: single-user,
// applied on its own, a percentage off every order its members place.
func tierPromotion(code string, fraction float64) domain.Promotion {
	return domain.Promotion{
		Code:      code,
		Scope:     domain.ScopeSingleUser,
		ApplyMode: domain.ApplyModeAuto,
		Active:    true,
		Benefit: domain.Benefit{
			Target:           domain.BenefitTargetGoods,
			DiscountType:     domain.DiscountTypePercent,
			DiscountFraction: fraction,
			ApplyTo:          domain.ApplyToEntireSubtotal,
		},
	}
}


func TestTierAppliesToEveryOrderOfAMember(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	if _, err := svc.Create(ctx, tierPromotion("VIP", 0.10)); err != nil {
		t.Fatalf("create VIP: %v", err)
	}
	if _, err := svc.Issue(ctx, "VIP", "cust-1", "issue", "manager:cust-1", "mgr"); err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// No ledger, no cap: the same member earns it order after order.
	for i := 0; i < 3; i++ {
		got, err := svc.EvaluateTier(ctx, cartFor("cust-1", 200000))
		if err != nil {
			t.Fatalf("EvaluateTier: %v", err)
		}
		if !got.OK || got.Code != "VIP" || got.DiscountWon != 20000 {
			t.Fatalf("order %d: got %+v, want VIP 20000", i, got)
		}
	}

	other, err := svc.EvaluateTier(ctx, cartFor("cust-2", 200000))
	if err != nil {
		t.Fatalf("EvaluateTier: %v", err)
	}
	if other.OK {
		t.Fatalf("a non-member earned a tier: %+v", other)
	}
}

// A member of two tiers gets the better one, not both.
func TestTierPicksTheBestOfSeveral(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	for _, p := range []domain.Promotion{tierPromotion("PRIVATE", 0.05), tierPromotion("VIP", 0.10)} {
		if _, err := svc.Create(ctx, p); err != nil {
			t.Fatalf("create %s: %v", p.Code, err)
		}
		if _, err := svc.Issue(ctx, p.Code, "cust-1", "issue", "manager:cust-1", "mgr"); err != nil {
			t.Fatalf("Issue %s: %v", p.Code, err)
		}
	}
	got, err := svc.EvaluateTier(ctx, cartFor("cust-1", 100000))
	if err != nil {
		t.Fatalf("EvaluateTier: %v", err)
	}
	if got.Code != "VIP" || got.DiscountWon != 10000 {
		t.Fatalf("got %+v, want VIP 10000", got)
	}
}

// Revoking the entitlement takes a customer out of the tier.
func TestRevokedTierNoLongerApplies(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	if _, err := svc.Create(ctx, tierPromotion("VIP", 0.10)); err != nil {
		t.Fatalf("create: %v", err)
	}
	entitlement, err := svc.Issue(ctx, "VIP", "cust-1", "issue", "manager:cust-1", "mgr")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if err := svc.Revoke(ctx, entitlement.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	got, err := svc.EvaluateTier(ctx, cartFor("cust-1", 100000))
	if err != nil {
		t.Fatalf("EvaluateTier: %v", err)
	}
	if got.OK {
		t.Fatalf("a revoked member still earned the tier: %+v", got)
	}
}

// A tier is never typed: entering its code, redeeming it or finding it in the
// wallet would let a member spend it a second time in the code slot.
func TestTierIsNotACode(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	if _, err := svc.Create(ctx, tierPromotion("VIP", 0.10)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Issue(ctx, "VIP", "cust-1", "issue", "manager:cust-1", "mgr"); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if got := svc.Evaluate(ctx, "VIP", cartFor("cust-1", 100000)); got.OK || got.Reason != domain.ReasonInvalidCode {
		t.Fatalf("Evaluate(VIP) = %+v, want invalid_code", got)
	}
	if _, ok := svc.Redeem(ctx, "VIP"); ok {
		t.Fatal("Redeem(VIP) should not find a tier")
	}
	wallet, err := svc.Wallet(ctx, "cust-1", cartFor("cust-1", 100000))
	if err != nil {
		t.Fatalf("Wallet: %v", err)
	}
	if len(wallet) != 0 {
		t.Fatalf("wallet lists %d entries, want the tier left out", len(wallet))
	}
}

func TestTierDefinitionRules(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)

	global := tierPromotion("OPEN", 0.10)
	global.Scope = domain.ScopeGlobal
	if _, err := svc.Create(ctx, global); err == nil {
		t.Fatal("a global tier should be refused: membership is the entitlement")
	}

	capped := tierPromotion("CAPPED", 0.10)
	n := 100
	capped.MaxRedemptions = &n
	if _, err := svc.Create(ctx, capped); err == nil {
		t.Fatal("a tier with max_redemptions should be refused")
	}

	bad := tierPromotion("BAD", 0.10)
	bad.ApplyMode = "sometimes"
	if _, err := svc.Create(ctx, bad); err == nil {
		t.Fatal("an unknown apply_mode should be refused")
	}
}
