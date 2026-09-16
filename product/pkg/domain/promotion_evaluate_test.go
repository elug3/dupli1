package domain_test

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
)

func ptrInt64(v int64) *int64 { return &v }
func ptrInt(v int) *int       { return &v }

func line(skuID string, qty int, priceWon int64) domain.EvaluationLine {
	return domain.EvaluationLine{SkuID: skuID, SKU: skuID, Quantity: qty, UnitPriceWon: priceWon}
}

func cart(lines ...domain.EvaluationLine) domain.EvaluationContext {
	return domain.EvaluationContext{
		CustomerID:     "cust-1",
		ShippingFeeWon: 30000,
		Lines:          lines,
		Now:            time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC),
	}
}

// fixedWon is the sign-up campaign's shape: a flat won amount off goods.
func fixedWon(amount int64) domain.Benefit {
	return domain.Benefit{
		Target:           domain.BenefitTargetGoods,
		DiscountType:     domain.DiscountTypeFixed,
		DiscountFixedWon: amount,
		ApplyTo:          domain.ApplyToEntireSubtotal,
	}
}

func percent(fraction float64) domain.Benefit {
	return domain.Benefit{
		Target:           domain.BenefitTargetGoods,
		DiscountType:     domain.DiscountTypePercent,
		DiscountFraction: fraction,
		ApplyTo:          domain.ApplyToEntireSubtotal,
	}
}

func active(b domain.Benefit) domain.Promotion {
	return domain.Promotion{Code: "WELCOME", Scope: domain.ScopeGlobal, Active: true, Benefit: b}
}

// ── Benefit maths ────────────────────────────────────────────────────────────

func TestFixedBenefitDiscountsTheFlatAmount(t *testing.T) {
	got := active(fixedWon(5000)).Evaluate(cart(line("sku-1", 1, 50000)))
	if !got.OK {
		t.Fatalf("expected eligible, got %s/%s", got.Reason, got.SubReason)
	}
	if got.DiscountWon != 5000 {
		t.Fatalf("discount = %d, want 5000", got.DiscountWon)
	}
}

// The campaign's central money rule: a flat discount can never exceed what the
// cart is worth, so an order total cannot go negative or dip below shipping.
func TestFixedBenefitIsClampedToASmallerCart(t *testing.T) {
	got := active(fixedWon(5000)).Evaluate(cart(line("sku-1", 1, 3000)))
	if !got.OK {
		t.Fatalf("expected eligible, got %s", got.Reason)
	}
	if got.DiscountWon != 3000 {
		t.Fatalf("discount = %d, want 3000 (clamped to subtotal)", got.DiscountWon)
	}
}

func TestPercentBenefitTruncatesRatherThanRounding(t *testing.T) {
	// 30% of 3333 is 999.9; paying out 1000 would over-discount by a won.
	got := active(percent(0.30)).Evaluate(cart(line("sku-1", 1, 3333)))
	if got.DiscountWon != 999 {
		t.Fatalf("discount = %d, want 999", got.DiscountWon)
	}
}

func TestMaxDiscountCapsAPercentage(t *testing.T) {
	b := percent(0.50)
	b.MaxDiscountWon = ptrInt64(20000)
	got := active(b).Evaluate(cart(line("sku-1", 1, 100000)))
	if got.DiscountWon != 20000 {
		t.Fatalf("discount = %d, want 20000 (capped)", got.DiscountWon)
	}
}

func TestDiscountUsesQuantityExtendedPrice(t *testing.T) {
	got := active(percent(0.10)).Evaluate(cart(line("sku-1", 3, 10000)))
	if got.EligibleSubtotalWon != 30000 {
		t.Fatalf("base = %d, want 30000", got.EligibleSubtotalWon)
	}
	if got.DiscountWon != 3000 {
		t.Fatalf("discount = %d, want 3000", got.DiscountWon)
	}
}

// ── Minimum spend, the campaign's only required predicate ────────────────────

func minSpend(won float64) domain.Conditions {
	return domain.Conditions{
		Version: domain.ConditionsVersion,
		All: []domain.Predicate{
			{Attr: domain.AttrSubtotalWon, Op: domain.OpGte, Value: won},
		},
	}
}

func TestMinimumSpendBoundary(t *testing.T) {
	promo := active(fixedWon(5000))
	promo.Conditions = minSpend(100000)

	cases := []struct {
		name     string
		subtotal int64
		wantOK   bool
	}{
		{"just under", 99999, false},
		{"exactly at the threshold", 100000, true},
		{"just over", 100001, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := promo.Evaluate(cart(line("sku-1", 1, tc.subtotal)))
			if got.OK != tc.wantOK {
				t.Fatalf("ok = %v, want %v (reason %s/%s)", got.OK, tc.wantOK, got.Reason, got.SubReason)
			}
			if !tc.wantOK && got.SubReason != domain.SubReasonMinSpend {
				t.Fatalf("sub_reason = %q, want %q", got.SubReason, domain.SubReasonMinSpend)
			}
		})
	}
}

// ── Expiry ───────────────────────────────────────────────────────────────────

func TestExpiryBoundaryInKST(t *testing.T) {
	expires, err := domain.EndOfDayKST("2026-08-31")
	if err != nil {
		t.Fatalf("EndOfDayKST: %v", err)
	}
	promo := active(fixedWon(5000))
	promo.ExpiresAt = &expires

	// 23:59 on the 31st in Seoul is still inside the window...
	stillValid := time.Date(2026, 8, 31, 23, 59, 0, 0, domain.KST)
	// ...and one second past midnight on the 1st is not.
	justExpired := time.Date(2026, 9, 1, 0, 0, 1, 0, domain.KST)

	ctx := cart(line("sku-1", 1, 50000))
	ctx.Now = stillValid
	if got := promo.Evaluate(ctx); !got.OK {
		t.Fatalf("23:59 KST on the expiry date should still apply, got %s", got.Reason)
	}
	ctx.Now = justExpired
	if got := promo.Evaluate(ctx); got.Reason != domain.ReasonExpired {
		t.Fatalf("past midnight KST should be expired, got ok=%v reason=%s", got.OK, got.Reason)
	}
}

func TestNoExpiryNeverExpires(t *testing.T) {
	ctx := cart(line("sku-1", 1, 50000))
	ctx.Now = time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := active(fixedWon(5000)).Evaluate(ctx); !got.OK {
		t.Fatalf("a code with no expires_at should never expire, got %s", got.Reason)
	}
}

// The free-text legacy column was never comparable, so it must not be treated
// as an expiry — a definition has to be given a real ExpiresAt to be enforced.
func TestLegacyExpiresTextIsNotEnforced(t *testing.T) {
	promo := active(fixedWon(5000))
	promo.Expires = "Aug 31, 2026"
	ctx := cart(line("sku-1", 1, 50000))
	ctx.Now = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := promo.Evaluate(ctx); !got.OK {
		t.Fatalf("legacy free-text expires must not gate, got %s", got.Reason)
	}
}

// ── Inactive, exhausted, empty ───────────────────────────────────────────────

// An inactive code is reported as invalid, not "paused": distinguishing them
// would let anyone enumerate which campaigns exist.
func TestInactiveCodeLooksLikeAnUnknownCode(t *testing.T) {
	promo := active(fixedWon(5000))
	promo.Active = false
	if got := promo.Evaluate(cart(line("sku-1", 1, 50000))); got.Reason != domain.ReasonInvalidCode {
		t.Fatalf("reason = %s, want invalid_code", got.Reason)
	}
}

func TestCampaignCapExhausts(t *testing.T) {
	promo := active(fixedWon(5000))
	promo.MaxRedemptions = ptrInt(2)
	promo.RedemptionCount = 2
	if got := promo.Evaluate(cart(line("sku-1", 1, 50000))); got.Reason != domain.ReasonCampaignExhausted {
		t.Fatalf("reason = %s, want campaign_exhausted", got.Reason)
	}
}

func TestEmptyCartIsNotEligible(t *testing.T) {
	if got := active(fixedWon(5000)).Evaluate(cart()); got.OK {
		t.Fatal("an empty cart should not earn a discount")
	}
}

// ── Line predicates ──────────────────────────────────────────────────────────

func TestCategoryPredicateSelectsEligibleLines(t *testing.T) {
	promo := active(percent(0.10))
	promo.Benefit.ApplyTo = domain.ApplyToEligibleLines
	promo.Conditions = domain.Conditions{
		Version: domain.ConditionsVersion,
		All: []domain.Predicate{
			{Attr: domain.AttrLineCategory, Op: domain.OpIn, Value: []any{"bags"}},
		},
	}

	bag := line("sku-bag", 1, 100000)
	bag.Category = "bags"
	wallet := line("sku-wallet", 1, 50000)
	wallet.Category = "wallets"

	got := promo.Evaluate(cart(bag, wallet))
	if !got.OK {
		t.Fatalf("expected eligible, got %s/%s", got.Reason, got.SubReason)
	}
	// Only the bag counts toward the base, so the wallet stays full price.
	if got.EligibleSubtotalWon != 100000 {
		t.Fatalf("eligible base = %d, want 100000", got.EligibleSubtotalWon)
	}
	if got.DiscountWon != 10000 {
		t.Fatalf("discount = %d, want 10000", got.DiscountWon)
	}
	if len(got.EligibleSkuIDs) != 1 || got.EligibleSkuIDs[0] != "sku-bag" {
		t.Fatalf("eligible skus = %v, want [sku-bag]", got.EligibleSkuIDs)
	}
}

func TestNoMatchingCategoryIsNotEligible(t *testing.T) {
	promo := active(percent(0.10))
	promo.Conditions = domain.Conditions{
		Version: domain.ConditionsVersion,
		All: []domain.Predicate{
			{Attr: domain.AttrLineCategory, Op: domain.OpIn, Value: []any{"bags"}},
		},
	}
	wallet := line("sku-wallet", 1, 50000)
	wallet.Category = "wallets"

	got := promo.Evaluate(cart(wallet))
	if got.OK {
		t.Fatal("a cart with no matching category should not be eligible")
	}
	if got.SubReason != domain.SubReasonCategory {
		t.Fatalf("sub_reason = %q, want %q", got.SubReason, domain.SubReasonCategory)
	}
}

func TestLineMatchAllRequiresEveryLine(t *testing.T) {
	promo := active(percent(0.10))
	promo.Conditions = domain.Conditions{
		Version:   domain.ConditionsVersion,
		LineMatch: domain.LineMatchAll,
		All: []domain.Predicate{
			{Attr: domain.AttrLineBrandCode, Op: domain.OpEq, Value: "PRADA"},
		},
	}
	prada := line("sku-1", 1, 100000)
	prada.BrandCode = "PRADA"
	gucci := line("sku-2", 1, 100000)
	gucci.BrandCode = "GUCCI"

	if got := promo.Evaluate(cart(prada)); !got.OK {
		t.Fatalf("all-PRADA cart should be eligible, got %s", got.Reason)
	}
	if got := promo.Evaluate(cart(prada, gucci)); got.OK {
		t.Fatal("a mixed-brand cart should fail line_match=all")
	}
}

func TestOnSaleExclusion(t *testing.T) {
	promo := active(percent(0.10))
	promo.Conditions = domain.Conditions{
		Version: domain.ConditionsVersion,
		Exclude: []domain.Predicate{
			{Attr: domain.AttrLineOnSale, Op: domain.OpEq, Value: true},
		},
	}
	marked := line("sku-sale", 1, 50000)
	marked.OnSale = true

	got := promo.Evaluate(cart(marked))
	if got.OK {
		t.Fatal("an all-on-sale cart should be excluded")
	}
	if got.SubReason != domain.SubReasonOnSale {
		t.Fatalf("sub_reason = %q, want %q", got.SubReason, domain.SubReasonOnSale)
	}
}

// ── First-order-only fails closed ────────────────────────────────────────────

func TestFirstOrderOnlyPredicate(t *testing.T) {
	promo := active(fixedWon(5000))
	promo.Conditions = domain.Conditions{
		Version: domain.ConditionsVersion,
		All: []domain.Predicate{
			{Attr: domain.AttrCustomerPaidOrderCount, Op: domain.OpEq, Value: float64(0)},
		},
	}

	ctx := cart(line("sku-1", 1, 50000))
	ctx.PaidOrderCount = ptrInt(0)
	if got := promo.Evaluate(ctx); !got.OK {
		t.Fatalf("a first-time buyer should qualify, got %s/%s", got.Reason, got.SubReason)
	}

	ctx.PaidOrderCount = ptrInt(1)
	if got := promo.Evaluate(ctx); got.OK {
		t.Fatal("a returning buyer should not qualify")
	}

	// An unknown history must not pay out — the caller could not prove the
	// customer is new, so the safe answer is no.
	ctx.PaidOrderCount = nil
	if got := promo.Evaluate(ctx); got.OK {
		t.Fatal("an unknown paid-order count must fail closed, not grant the discount")
	}
}

// ── Legacy rows still price ──────────────────────────────────────────────────

func TestPrePhase2RowPricesFromTheLegacyDiscountColumn(t *testing.T) {
	promo := domain.Promotion{Code: "SUMMER30", Active: true, Discount: 0.30}
	got := promo.Evaluate(cart(line("sku-1", 1, 100000)))
	if !got.OK {
		t.Fatalf("a legacy row should still apply, got %s", got.Reason)
	}
	if got.DiscountWon != 30000 {
		t.Fatalf("discount = %d, want 30000", got.DiscountWon)
	}
}

func TestAppliedBenefitSnapshotIsReturned(t *testing.T) {
	got := active(fixedWon(5000)).Evaluate(cart(line("sku-1", 1, 50000)))
	if got.AppliedBenefit == nil {
		t.Fatal("an applied benefit snapshot is required for the ledger")
	}
	if got.AppliedBenefit.DiscountFixedWon != 5000 {
		t.Fatalf("snapshot fixed won = %d, want 5000", got.AppliedBenefit.DiscountFixedWon)
	}
}
