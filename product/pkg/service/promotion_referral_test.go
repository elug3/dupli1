package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

// referralCode is SER-105's example: 15% off the goods for 허경호's
// acquaintances, 5% commission.
func referralCode() domain.Promotion {
	return domain.Promotion{
		Code:   "HUH15",
		Scope:  domain.ScopeGlobal,
		Active: true,
		Benefit: domain.Benefit{
			Target:           domain.BenefitTargetGoods,
			DiscountType:     domain.DiscountTypePercent,
			DiscountFraction: 0.15,
			ApplyTo:          domain.ApplyToEntireSubtotal,
		},
		Kind:          domain.PromotionKindReferral,
		ReferrerName:  "  허경호 ",
		CommissionBps: 500,
	}
}

func TestReferralCodeCanBeUsedOnEveryOrder(t *testing.T) {
	svc, _ := newPromotionSvc(t)
	ctx := context.Background()
	created, err := svc.Create(ctx, referralCode())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ReferrerName != "허경호" {
		t.Fatalf("referrer name = %q, want trimmed", created.ReferrerName)
	}

	for _, orderID := range []string{"ord-1", "ord-2", "ord-3"} {
		cart := cartFor("cust-1", 1_000_000)
		if got := svc.Evaluate(ctx, "huh15", cart); !got.OK || got.DiscountWon != 150_000 {
			t.Fatalf("%s evaluate = %+v, want ok with 150,000 off the goods", orderID, got)
		}
		res, err := svc.ReserveForOrder(ctx, "HUH15", orderID, cart)
		if err != nil || !res.Result.OK {
			t.Fatalf("%s reserve = %+v, %v; a referral code has no per-customer limit", orderID, res.Result, err)
		}
		if res.Referral == nil || res.Referral.ReferrerName != "허경호" || res.Referral.CommissionBps != 500 {
			t.Fatalf("%s referral = %+v, want 허경호 at 500 bps", orderID, res.Referral)
		}
		if err := svc.Consume(ctx, orderID); err != nil {
			t.Fatalf("consume: %v", err)
		}
	}
}

func TestOrdinaryCodeReservationCarriesNoReferral(t *testing.T) {
	svc, _ := newPromotionSvc(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, fixedPromotion("SAVE5K", 5000)); err != nil {
		t.Fatalf("create: %v", err)
	}
	res, err := svc.ReserveForOrder(ctx, "SAVE5K", "ord-1", cartFor("cust-1", 100_000))
	if err != nil || !res.Result.OK {
		t.Fatalf("reserve = %+v, %v", res.Result, err)
	}
	if res.Referral != nil {
		t.Fatalf("referral = %+v, want nil for an ordinary code", res.Referral)
	}
	// And an ordinary code is still once per customer.
	res, _ = svc.ReserveForOrder(ctx, "SAVE5K", "ord-2", cartFor("cust-1", 100_000))
	if res.Result.OK || res.Result.Reason != domain.ReasonAlreadyUsed {
		t.Fatalf("second use = %+v, want already_used", res.Result)
	}
}

func TestReferralCodeValidation(t *testing.T) {
	cases := map[string]func(p *domain.Promotion){
		"no referrer name":             func(p *domain.Promotion) { p.ReferrerName = " " },
		"commission over 100%":         func(p *domain.Promotion) { p.CommissionBps = 10001 },
		"negative commission":          func(p *domain.Promotion) { p.CommissionBps = -1 },
		"single user":                  func(p *domain.Promotion) { p.Scope = domain.ScopeSingleUser },
		"unknown kind":                 func(p *domain.Promotion) { p.Kind = "partner" },
		"name too long":                func(p *domain.Promotion) { p.ReferrerName = strings.Repeat("가", domain.MaxReferrerNameLen+1) },
		"referrer on an ordinary code": func(p *domain.Promotion) { p.Kind = domain.PromotionKindStandard },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			svc, _ := newPromotionSvc(t)
			p := referralCode()
			mutate(&p)
			_, err := svc.Create(context.Background(), p)
			if !errors.Is(err, ports.ErrInvalid) {
				t.Fatalf("create err = %v, want invalid", err)
			}
		})
	}
}

func TestReferralCommissionCanBeEditedButNotCleared(t *testing.T) {
	svc, _ := newPromotionSvc(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, referralCode()); err != nil {
		t.Fatalf("create: %v", err)
	}
	rate := 700
	name := "허경호 (지인)"
	updated, err := svc.Update(ctx, "HUH15", ports.PromotionPatch{CommissionBps: &rate, ReferrerName: &name})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.CommissionBps != 700 || updated.ReferrerName != name || !updated.IsReferral() {
		t.Fatalf("updated = %+v", updated)
	}
	empty := ""
	if _, err := svc.Update(ctx, "HUH15", ports.PromotionPatch{ReferrerName: &empty}); !errors.Is(err, ports.ErrInvalid) {
		t.Fatalf("clearing the referrer err = %v, want invalid", err)
	}
}

func TestPublicViewHidesTheReferral(t *testing.T) {
	p := referralCode()
	public := p.PublicView()
	if public.Kind != "" || public.ReferrerName != "" || public.CommissionBps != 0 {
		t.Fatalf("public view = %+v, want no referral fields", public)
	}
	if public.Benefit.DiscountFraction != 0.15 {
		t.Fatal("public view must keep the benefit")
	}
}
