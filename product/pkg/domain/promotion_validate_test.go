package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
)

// Before Phase 2 nothing validated a definition on write: a discount outside
// (0,1) saved cleanly and then failed at a customer's checkout, where nobody
// who could fix it would ever see it. These pin the write-time gate.

func TestBenefitValidateRejectsBadPercentages(t *testing.T) {
	cases := []struct {
		name     string
		fraction float64
	}{
		{"zero", 0},
		{"negative", -0.1},
		{"one", 1},
		{"greater than one", 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := domain.Benefit{
				Target:           domain.BenefitTargetGoods,
				DiscountType:     domain.DiscountTypePercent,
				DiscountFraction: tc.fraction,
			}
			if err := b.Validate(); err == nil {
				t.Fatalf("fraction %v should be rejected on write", tc.fraction)
			}
		})
	}
}

func TestBenefitValidateAcceptsAValidPercentage(t *testing.T) {
	b := domain.Benefit{
		Target:           domain.BenefitTargetGoods,
		DiscountType:     domain.DiscountTypePercent,
		DiscountFraction: 0.30,
	}
	if err := b.Validate(); err != nil {
		t.Fatalf("30%% off goods should be valid: %v", err)
	}
}

func TestBenefitValidateRejectsNonPositiveFixedAmount(t *testing.T) {
	for _, amount := range []int64{0, -1} {
		b := domain.Benefit{
			Target:           domain.BenefitTargetGoods,
			DiscountType:     domain.DiscountTypeFixed,
			DiscountFixedWon: amount,
		}
		if err := b.Validate(); err == nil {
			t.Fatalf("fixed amount %d should be rejected", amount)
		}
	}
}

func TestBenefitValidateRejectsMixingPercentAndFixed(t *testing.T) {
	b := domain.Benefit{
		Target:           domain.BenefitTargetGoods,
		DiscountType:     domain.DiscountTypeFixed,
		DiscountFixedWon: 5000,
		DiscountFraction: 0.3,
	}
	if err := b.Validate(); err == nil {
		t.Fatal("a fixed benefit carrying a fraction should be rejected")
	}
}

// Shipping targets are Phase 4. Accepting one now would save a definition that
// silently discounts nothing, so it is rejected with a message that says why.
func TestBenefitValidateRejectsUnimplementedTargetsClearly(t *testing.T) {
	for _, target := range []domain.BenefitTarget{
		domain.BenefitTargetShipping,
		domain.BenefitTargetGoodsAndShipping,
		domain.BenefitTargetNone,
	} {
		b := domain.Benefit{
			Target:           target,
			DiscountType:     domain.DiscountTypePercent,
			DiscountFraction: 0.3,
		}
		err := b.Validate()
		if err == nil {
			t.Fatalf("target %q should be rejected until Phase 4", target)
		}
		if !strings.Contains(err.Error(), "not implemented yet") {
			t.Fatalf("target %q should say it is unimplemented, got %q", target, err)
		}
	}
}

func TestBenefitValidateRejectsNonPositiveCap(t *testing.T) {
	zero := int64(0)
	b := domain.Benefit{
		Target:           domain.BenefitTargetGoods,
		DiscountType:     domain.DiscountTypePercent,
		DiscountFraction: 0.3,
		MaxDiscountWon:   &zero,
	}
	if err := b.Validate(); err == nil {
		t.Fatal("a zero cap should be rejected")
	}
}

// ── Conditions ───────────────────────────────────────────────────────────────

func TestEmptyConditionsAreValidAndAlwaysEligible(t *testing.T) {
	var c domain.Conditions
	if err := c.Validate(); err != nil {
		t.Fatalf("an empty document should be valid: %v", err)
	}
	if !c.IsEmpty() {
		t.Fatal("an empty document should report IsEmpty")
	}
}

func TestConditionsRejectUnknownAttribute(t *testing.T) {
	c := domain.Conditions{
		Version: domain.ConditionsVersion,
		All:     []domain.Predicate{{Attr: "line.secret_margin", Op: domain.OpGte, Value: 1.0}},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("an attribute outside the allowlist should be rejected")
	}
}

// customer.paid_order_count was allowlisted before anything could supply it:
// only order knows the number, it sends none, and the predicate failed closed,
// so a manager could author a rule that refused every cart. Removed on
// 2026-09-21 rather than left as a condition nothing could satisfy.
func TestConditionsRejectThePaidOrderCountAttribute(t *testing.T) {
	c := domain.Conditions{
		Version: domain.ConditionsVersion,
		All: []domain.Predicate{
			{Attr: "customer.paid_order_count", Op: domain.OpEq, Value: 0.0},
		},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("customer.paid_order_count is no longer an allowed attribute")
	}
}

func TestConditionsRejectUnknownOp(t *testing.T) {
	c := domain.Conditions{
		Version: domain.ConditionsVersion,
		All:     []domain.Predicate{{Attr: domain.AttrSubtotalWon, Op: "regex", Value: ".*"}},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("an unknown op should be rejected")
	}
}

func TestConditionsRejectOrderingAStringAttribute(t *testing.T) {
	c := domain.Conditions{
		Version: domain.ConditionsVersion,
		All:     []domain.Predicate{{Attr: domain.AttrLineCategory, Op: domain.OpGte, Value: "bags"}},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("gte on a string attribute should be rejected")
	}
}

func TestConditionsRejectListValueForScalarOp(t *testing.T) {
	c := domain.Conditions{
		Version: domain.ConditionsVersion,
		All:     []domain.Predicate{{Attr: domain.AttrSubtotalWon, Op: domain.OpGte, Value: []any{1.0, 2.0}}},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("a list value for gte should be rejected")
	}
}

func TestConditionsRejectScalarValueForSetOp(t *testing.T) {
	c := domain.Conditions{
		Version: domain.ConditionsVersion,
		All:     []domain.Predicate{{Attr: domain.AttrLineCategory, Op: domain.OpIn, Value: "bags"}},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("a scalar value for in should be rejected")
	}
}

func TestConditionsRejectUnsupportedVersion(t *testing.T) {
	c := domain.Conditions{
		Version: 99,
		All:     []domain.Predicate{{Attr: domain.AttrSubtotalWon, Op: domain.OpGte, Value: 1.0}},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("an unknown document version should be rejected, not guessed at")
	}
}

func TestConditionsRejectMissingValue(t *testing.T) {
	c := domain.Conditions{
		Version: domain.ConditionsVersion,
		All:     []domain.Predicate{{Attr: domain.AttrSubtotalWon, Op: domain.OpGte}},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("a predicate with no value should be rejected")
	}
}

func TestConditionsAcceptTheSignUpCampaignShape(t *testing.T) {
	c := domain.Conditions{
		Version: domain.ConditionsVersion,
		All: []domain.Predicate{
			{Attr: domain.AttrSubtotalWon, Op: domain.OpGte, Value: 100000.0},
		},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("the campaign's min-spend document should be valid: %v", err)
	}
}

// ── KST authoring ────────────────────────────────────────────────────────────

func TestEndOfDayKSTIsTheLastInstantOfTheDayInSeoul(t *testing.T) {
	got, err := domain.EndOfDayKST("2026-08-31")
	if err != nil {
		t.Fatalf("EndOfDayKST: %v", err)
	}
	// 23:59:59.999999999 KST is 14:59:59.999999999 UTC the same day.
	inKST := got.In(domain.KST)
	if inKST.Year() != 2026 || inKST.Month() != 8 || inKST.Day() != 31 {
		t.Fatalf("expiry lands on %s, want 2026-08-31 in KST", inKST.Format("2006-01-02"))
	}
	if inKST.Hour() != 23 || inKST.Minute() != 59 || inKST.Second() != 59 {
		t.Fatalf("expiry at %s, want 23:59:59 KST", inKST.Format("15:04:05"))
	}
	if got.Location() != time.UTC {
		t.Fatalf("expiry should be stored as UTC, got %s", got.Location())
	}
}

func TestEndOfDayKSTRejectsRubbish(t *testing.T) {
	if _, err := domain.EndOfDayKST("31/08/2026"); err == nil {
		t.Fatal("a non-ISO date should be rejected")
	}
}
