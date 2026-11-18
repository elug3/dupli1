package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

// tierPromotion is a customer tier (apply_mode auto) as one was registered
// before tiers were retired for the points multiplier.
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

// No new tier can be made, either by creating one or by switching a code.
func TestTiersAreRetired(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	if _, err := svc.Create(ctx, tierPromotion("VIP", 0.10)); !errors.Is(err, ports.ErrInvalid) {
		t.Fatalf("Create(auto) err = %v, want invalid", err)
	}

	code := tierPromotion("PRIVATE", 0.10)
	code.ApplyMode = domain.ApplyModeCode
	if _, err := svc.Create(ctx, code); err != nil {
		t.Fatalf("Create(code): %v", err)
	}
	auto := domain.ApplyModeAuto
	if _, err := svc.Update(ctx, "PRIVATE", ports.PromotionPatch{ApplyMode: &auto}); !errors.Is(err, ports.ErrInvalid) {
		t.Fatalf("Update to auto err = %v, want invalid", err)
	}

	bad := tierPromotion("BAD", 0.10)
	bad.ApplyMode = "sometimes"
	if _, err := svc.Create(ctx, bad); err == nil {
		t.Fatal("an unknown apply_mode should be refused")
	}
}

// A tier stored before the retirement is inert: not typeable, not redeemable,
// not in the wallet. A manager can still edit it, or switch it to a code.
func TestStoredTierIsInert(t *testing.T) {
	ctx := context.Background()
	svc, store := newPromotionSvc(t)
	if err := store.Create(ctx, tierPromotion("VIP", 0.10)); err != nil {
		t.Fatalf("store.Create: %v", err)
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

	off := false
	if _, err := svc.Update(ctx, "VIP", ports.PromotionPatch{Active: &off}); err != nil {
		t.Fatalf("deactivating a stored tier: %v", err)
	}
	code := domain.ApplyModeCode
	updated, err := svc.Update(ctx, "VIP", ports.PromotionPatch{ApplyMode: &code})
	if err != nil {
		t.Fatalf("switching a stored tier to code: %v", err)
	}
	if updated.IsTier() {
		t.Fatal("still a tier after switching to code")
	}
}
