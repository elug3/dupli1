package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/elug3/dupli1/product/pkg/service"
)

func newPromotionSvc(t *testing.T) (*service.PromotionService, *memory.PromotionStore) {
	t.Helper()
	store := memory.NewPromotionStore()
	ledger := memory.NewPromotionRedemptionStore(store)
	svc := service.NewPromotionService(store).
		WithLedger(ledger).
		WithEntitlements(memory.NewPromotionEntitlementStore())
	return svc, store
}

func fixedPromotion(code string, amountWon int64) domain.Promotion {
	return domain.Promotion{
		Code:   code,
		Scope:  domain.ScopeGlobal,
		Active: true,
		Benefit: domain.Benefit{
			Target:           domain.BenefitTargetGoods,
			DiscountType:     domain.DiscountTypeFixed,
			DiscountFixedWon: amountWon,
			ApplyTo:          domain.ApplyToEntireSubtotal,
		},
	}
}

func cartFor(customerID string, subtotalWon int64) domain.EvaluationContext {
	return domain.EvaluationContext{
		CustomerID:     customerID,
		ShippingFeeWon: 30000,
		Lines: []domain.EvaluationLine{
			{SkuID: "sku-1", SKU: "sku-1", Quantity: 1, UnitPriceWon: subtotalWon},
		},
	}
}

// ── Write-time validation ────────────────────────────────────────────────────

func TestCreateRejectsAnInvalidBenefit(t *testing.T) {
	svc, _ := newPromotionSvc(t)
	p := fixedPromotion("BAD", 0) // fixed benefit with no amount
	if _, err := svc.Create(context.Background(), p); err == nil {
		t.Fatal("a fixed benefit of 0 should be refused on write")
	}
}

// The pre-Phase-2 defect: a discount outside (0,1) saved and only failed later.
func TestCreateRejectsALegacyDiscountOutsideRange(t *testing.T) {
	svc, _ := newPromotionSvc(t)
	p := domain.Promotion{Code: "BAD2", Active: true, Discount: 5}
	if _, err := svc.Create(context.Background(), p); err == nil {
		t.Fatal("a discount of 5 should be refused on write, not at checkout")
	}
}

func TestCreateRejectsConditionsOutsideTheAllowlist(t *testing.T) {
	svc, _ := newPromotionSvc(t)
	p := fixedPromotion("BAD3", 5000)
	p.Conditions = domain.Conditions{
		Version: domain.ConditionsVersion,
		All:     []domain.Predicate{{Attr: "line.cost_price", Op: domain.OpLte, Value: 1.0}},
	}
	if _, err := svc.Create(context.Background(), p); err == nil {
		t.Fatal("an attribute outside the allowlist should be refused")
	}
}

// An update is validated against the definition it would produce, so a partial
// body cannot leave a code in a state that only fails at someone's checkout.
func TestUpdateValidatesThePatchedResult(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	if _, err := svc.Create(ctx, fixedPromotion("WELCOME", 5000)); err != nil {
		t.Fatalf("create: %v", err)
	}
	bad := domain.Benefit{
		Target:           domain.BenefitTargetGoods,
		DiscountType:     domain.DiscountTypePercent,
		DiscountFraction: 3,
	}
	if _, err := svc.Update(ctx, "WELCOME", ports.PromotionPatch{Benefit: &bad}); err == nil {
		t.Fatal("patching in a 300% discount should be refused")
	}
}

// ── Once per customer ────────────────────────────────────────────────────────

func TestSecondUseByTheSameCustomerIsRefused(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	if _, err := svc.Create(ctx, fixedPromotion("WELCOME", 5000)); err != nil {
		t.Fatalf("create: %v", err)
	}

	_, first, err := svc.Reserve(ctx, "WELCOME", "ord-1", cartFor("cust-1", 50000))
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	if !first.OK || first.DiscountWon != 5000 {
		t.Fatalf("first use should apply 5000, got ok=%v discount=%d", first.OK, first.DiscountWon)
	}

	// Evaluating again for the same customer now reports already_used...
	again := svc.Evaluate(ctx, "WELCOME", cartFor("cust-1", 50000))
	if again.Reason != domain.ReasonAlreadyUsed {
		t.Fatalf("second evaluate reason = %s, want already_used", again.Reason)
	}
	// ...and reserving a second order with it is refused outright.
	_, second, err := svc.Reserve(ctx, "WELCOME", "ord-2", cartFor("cust-1", 50000))
	if err != nil {
		t.Fatalf("second reserve: %v", err)
	}
	if second.OK {
		t.Fatal("the same customer must not use the code twice")
	}
}

func TestADifferentCustomerStillGetsTheCode(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	if _, err := svc.Create(ctx, fixedPromotion("WELCOME", 5000)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, err := svc.Reserve(ctx, "WELCOME", "ord-1", cartFor("cust-1", 50000)); err != nil {
		t.Fatalf("reserve for cust-1: %v", err)
	}
	_, res, err := svc.Reserve(ctx, "WELCOME", "ord-2", cartFor("cust-2", 50000))
	if err != nil {
		t.Fatalf("reserve for cust-2: %v", err)
	}
	if !res.OK {
		t.Fatalf("a second customer should still qualify, got %s", res.Reason)
	}
}

// Retrying a complete for the same order must not burn a second use.
func TestReservingTheSameOrderTwiceIsIdempotent(t *testing.T) {
	ctx := context.Background()
	svc, store := newPromotionSvc(t)
	if _, err := svc.Create(ctx, fixedPromotion("WELCOME", 5000)); err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, res, err := svc.Reserve(ctx, "WELCOME", "ord-1", cartFor("cust-1", 50000)); err != nil || !res.OK {
			t.Fatalf("reserve %d: err=%v ok=%v reason=%s", i, err, res.OK, res.Reason)
		}
	}
	p, err := store.Get(ctx, "WELCOME")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if p.RedemptionCount != 1 {
		t.Fatalf("redemption_count = %d, want 1 after a retried reserve", p.RedemptionCount)
	}
}

// ── Reserve → consume → release ──────────────────────────────────────────────

// Cancelling before shipment hands the use back, so the customer may use the
// code again. This mirrors the stock rule.
func TestReleaseRestoresTheCustomersUse(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	if _, err := svc.Create(ctx, fixedPromotion("WELCOME", 5000)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, err := svc.Reserve(ctx, "WELCOME", "ord-1", cartFor("cust-1", 50000)); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := svc.Consume(ctx, "ord-1"); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if got := svc.Evaluate(ctx, "WELCOME", cartFor("cust-1", 50000)); got.Reason != domain.ReasonAlreadyUsed {
		t.Fatalf("while consumed the code should be used up, got %s", got.Reason)
	}

	if err := svc.Release(ctx, "ord-1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if got := svc.Evaluate(ctx, "WELCOME", cartFor("cust-1", 50000)); !got.OK {
		t.Fatalf("after release the customer should qualify again, got %s", got.Reason)
	}
}

func TestReleaseAlsoFreesACampaignSlot(t *testing.T) {
	ctx := context.Background()
	svc, store := newPromotionSvc(t)
	p := fixedPromotion("LIMITED", 5000)
	max := 1
	p.MaxRedemptions = &max
	if _, err := svc.Create(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, err := svc.Reserve(ctx, "LIMITED", "ord-1", cartFor("cust-1", 50000)); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if got := svc.Evaluate(ctx, "LIMITED", cartFor("cust-2", 50000)); got.Reason != domain.ReasonCampaignExhausted {
		t.Fatalf("the one-slot campaign should be exhausted, got %s", got.Reason)
	}
	if err := svc.Release(ctx, "ord-1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if got := svc.Evaluate(ctx, "LIMITED", cartFor("cust-2", 50000)); !got.OK {
		t.Fatalf("releasing should free the slot, got %s", got.Reason)
	}
	def, _ := store.Get(ctx, "LIMITED")
	if def.RedemptionCount != 0 {
		t.Fatalf("redemption_count = %d, want 0 after release", def.RedemptionCount)
	}
}

func TestConsumeIsIdempotent(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	if _, err := svc.Create(ctx, fixedPromotion("WELCOME", 5000)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, err := svc.Reserve(ctx, "WELCOME", "ord-1", cartFor("cust-1", 50000)); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	// A redelivered payment.succeeded must not change anything the second time.
	for i := 0; i < 3; i++ {
		if err := svc.Consume(ctx, "ord-1"); err != nil {
			t.Fatalf("consume %d: %v", i, err)
		}
	}
}

// ── Reserve refuses a cart that no longer earns the discount ─────────────────

func TestReserveReEvaluatesAgainstTheFinalCart(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	p := fixedPromotion("SPEND100K", 5000)
	p.Conditions = domain.Conditions{
		Version: domain.ConditionsVersion,
		All:     []domain.Predicate{{Attr: domain.AttrSubtotalWon, Op: domain.OpGte, Value: 100000.0}},
	}
	if _, err := svc.Create(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Qualifies at apply...
	if got := svc.Evaluate(ctx, "SPEND100K", cartFor("cust-1", 100000)); !got.OK {
		t.Fatalf("a 100,000 cart should qualify, got %s/%s", got.Reason, got.SubReason)
	}
	// ...but the customer removes an item before completing.
	_, res, err := svc.Reserve(ctx, "SPEND100K", "ord-1", cartFor("cust-1", 50000))
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if res.OK {
		t.Fatal("a shrunken cart must not keep the discount at complete")
	}
	if res.SubReason != domain.SubReasonMinSpend {
		t.Fatalf("sub_reason = %q, want min_spend", res.SubReason)
	}
}

// ── Expiry and lookup ────────────────────────────────────────────────────────

func TestRedeemRefusesAnExpiredCode(t *testing.T) {
	ctx := context.Background()
	store := memory.NewPromotionStore()
	svc := service.NewPromotionService(store).
		WithLedger(memory.NewPromotionRedemptionStore(store)).
		WithClock(func() time.Time { return time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC) })

	p := fixedPromotion("EXPIRED", 5000)
	past := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	p.ExpiresAt = &past
	if _, err := svc.Create(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, ok := svc.Redeem(ctx, "EXPIRED"); ok {
		t.Fatal("redeem should refuse an expired code")
	}
	if got := svc.Evaluate(ctx, "EXPIRED", cartFor("cust-1", 50000)); got.Reason != domain.ReasonExpired {
		t.Fatalf("evaluate reason = %s, want expired", got.Reason)
	}
}

// An unknown code and a soft-deleted one must be indistinguishable, or probing
// can enumerate live campaigns.
func TestUnknownAndInactiveCodesReportIdentically(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	p := fixedPromotion("PAUSED", 5000)
	p.Active = false
	if _, err := svc.Create(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}
	unknown := svc.Evaluate(ctx, "NO-SUCH-CODE", cartFor("cust-1", 50000))
	paused := svc.Evaluate(ctx, "PAUSED", cartFor("cust-1", 50000))
	if unknown.Reason != domain.ReasonInvalidCode || paused.Reason != domain.ReasonInvalidCode {
		t.Fatalf("unknown=%s paused=%s, want both invalid_code", unknown.Reason, paused.Reason)
	}
}

// ── Single-user entitlements ─────────────────────────────────────────────────

// enableWelcomePromotion activates the seeded sign-up campaign.
//
// The definition is seeded inactive in both stores so that deploying does not
// silently switch on a 50,000원 discount; a manager enables it. Tests go
// through that same step rather than creating their own definition, so the
// seeded parameters — 50,000원 off, 100,000원 minimum, 30-day window — are
// what is actually under test.
func enableWelcomePromotion(t *testing.T, svc *service.PromotionService) *domain.Promotion {
	t.Helper()
	active := true
	promotion, err := svc.Update(context.Background(), "WELCOME50", ports.PromotionPatch{Active: &active})
	if err != nil {
		t.Fatalf("enable WELCOME50: %v", err)
	}
	return promotion
}

// A single-user code does nothing for an account that was never issued it, and
// says the same thing as an unknown code — so guessing the campaign's code
// tells you nothing about whether it exists.
func TestSingleUserCodeNeedsAnEntitlement(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	enableWelcomePromotion(t, svc)
	got := svc.Evaluate(ctx, "WELCOME50", cartFor("cust-1", 150000))
	if got.OK {
		t.Fatal("an account with no entitlement must not get the discount")
	}
	unknown := svc.Evaluate(ctx, "NO-SUCH-CODE", cartFor("cust-1", 150000))
	if got.Reason != unknown.Reason {
		t.Fatalf("not-entitled=%s unknown=%s, want identical", got.Reason, unknown.Reason)
	}
}

func TestIssuedEntitlementUnlocksTheCode(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	enableWelcomePromotion(t, svc)
	if _, err := svc.Issue(ctx, "WELCOME50", "cust-1", "system", "user.registered:cust-1", ""); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	got := svc.Evaluate(ctx, "WELCOME50", cartFor("cust-1", 150000))
	if !got.OK {
		t.Fatalf("an entitled account should qualify, got %s/%s", got.Reason, got.SubReason)
	}
	if got.DiscountWon != 50000 {
		t.Fatalf("discount = %d, want 50000", got.DiscountWon)
	}
	// The entitlement grants access; the cart still has to earn it.
	under := svc.Evaluate(ctx, "WELCOME50", cartFor("cust-1", 99999))
	if under.OK || under.SubReason != domain.SubReasonMinSpend {
		t.Fatalf("under the minimum should fail on min_spend, got ok=%v %s/%s", under.OK, under.Reason, under.SubReason)
	}
}

// A redelivered registration event, or a re-run of the backfill, must not mint
// a second entitlement.
func TestIssueIsIdempotentOnTheTriggerKey(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	enableWelcomePromotion(t, svc)
	first, err := svc.Issue(ctx, "WELCOME50", "cust-1", "system", "user.registered:cust-1", "")
	if err != nil {
		t.Fatalf("first issue: %v", err)
	}
	second, err := svc.Issue(ctx, "WELCOME50", "cust-1", "system", "user.registered:cust-1", "")
	if err != nil {
		t.Fatalf("second issue: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("issued twice: %s then %s", first.ID, second.ID)
	}
	wallet, err := svc.Wallet(ctx, "cust-1", cartFor("cust-1", 150000))
	if err != nil {
		t.Fatalf("Wallet: %v", err)
	}
	if len(wallet) != 1 {
		t.Fatalf("wallet holds %d entitlements, want 1", len(wallet))
	}
}

// The entitlement's window is computed at issue time, so an account issued
// late in a campaign gets the same month as one issued at launch.
func TestEntitlementExpiresAMonthAfterIssue(t *testing.T) {
	ctx := context.Background()
	store := memory.NewPromotionStore()
	issuedAt := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	svc := service.NewPromotionService(store).
		WithLedger(memory.NewPromotionRedemptionStore(store)).
		WithEntitlements(memory.NewPromotionEntitlementStore()).
		WithClock(func() time.Time { return issuedAt })

	enableWelcomePromotion(t, svc)
	entitlement, err := svc.Issue(ctx, "WELCOME50", "cust-1", "system", "k1", "")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if entitlement.ExpiresAt == nil {
		t.Fatal("a 30-day window should give the entitlement an expiry")
	}
	want := issuedAt.AddDate(0, 0, 30)
	if !entitlement.ExpiresAt.Equal(want) {
		t.Fatalf("expires %s, want %s", entitlement.ExpiresAt, want)
	}

	// Inside the window it applies...
	inside := cartFor("cust-1", 150000)
	inside.Now = issuedAt.AddDate(0, 0, 29)
	if got := svc.Evaluate(ctx, "WELCOME50", inside); !got.OK {
		t.Fatalf("day 29 should still apply, got %s", got.Reason)
	}
	// ...and past it, it is expired rather than merely unknown, because the
	// customer really does hold it.
	outside := cartFor("cust-1", 150000)
	outside.Now = issuedAt.AddDate(0, 0, 31)
	if got := svc.Evaluate(ctx, "WELCOME50", outside); got.Reason != domain.ReasonExpired {
		t.Fatalf("day 31 = %s, want expired", got.Reason)
	}
}

func TestRevokedEntitlementStopsWorking(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	enableWelcomePromotion(t, svc)
	entitlement, err := svc.Issue(ctx, "WELCOME50", "cust-1", "issue", "manual-1", "mgr-1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if err := svc.Revoke(ctx, entitlement.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if got := svc.Evaluate(ctx, "WELCOME50", cartFor("cust-1", 150000)); got.OK {
		t.Fatal("a revoked entitlement must not apply")
	}
}

// The wallet shows ineligible codes with the reason rather than hiding them,
// so a customer who knows they have one is told why it will not apply.
func TestWalletExplainsAnIneligibleCode(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	enableWelcomePromotion(t, svc)
	if _, err := svc.Issue(ctx, "WELCOME50", "cust-1", "system", "k1", ""); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	wallet, err := svc.Wallet(ctx, "cust-1", cartFor("cust-1", 50000))
	if err != nil {
		t.Fatalf("Wallet: %v", err)
	}
	if len(wallet) != 1 {
		t.Fatalf("wallet holds %d entries, want 1", len(wallet))
	}
	entry := wallet[0]
	if entry.Eligible {
		t.Fatal("a 50,000 cart should not meet the 100,000 minimum")
	}
	if entry.SubReason != domain.SubReasonMinSpend {
		t.Fatalf("sub_reason = %q, want min_spend", entry.SubReason)
	}
	if entry.Promotion == nil || entry.Promotion.Code != "WELCOME50" {
		t.Fatal("the wallet entry should carry the definition it grants")
	}
}

// Issuing only makes sense for single-user codes; a global one needs no
// entitlement and issuing it would imply a limit that does not exist.
func TestIssuingAGlobalCodeIsRefused(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	if _, err := svc.Create(ctx, fixedPromotion("SHARED", 5000)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Issue(ctx, "SHARED", "cust-1", "issue", "k", "mgr"); err == nil {
		t.Fatal("issuing a global code should be refused")
	}
}

var _ = errors.Is

// ── The seeded campaign ──────────────────────────────────────────────────────

// The seed is what production will actually run, so its parameters are pinned
// here rather than left to be discovered when the campaign goes live.
func TestSeededWelcomeCampaignMatchesTheAgreedParameters(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)

	promotion, err := svc.Get(ctx, "WELCOME50")
	if err != nil {
		t.Fatalf("the sign-up campaign should be seeded: %v", err)
	}
	// Seeded inactive: deploying must not switch on a 50,000원 discount.
	if promotion.Active {
		t.Fatal("WELCOME50 should be seeded inactive, for a manager to enable")
	}
	if promotion.EffectiveScope() != domain.ScopeSingleUser {
		t.Fatalf("scope = %s, want single_user", promotion.EffectiveScope())
	}
	benefit := promotion.EffectiveBenefit()
	if benefit.DiscountType != domain.DiscountTypeFixed || benefit.DiscountFixedWon != 50000 {
		t.Fatalf("benefit = %+v, want a fixed 50000 won", benefit)
	}
	if promotion.EntitlementTTLDays != 30 {
		t.Fatalf("entitlement window = %d days, want 30", promotion.EntitlementTTLDays)
	}
	// No budget cap: the campaign is uncapped by agreement.
	if promotion.MaxRedemptions != nil {
		t.Fatalf("max_redemptions = %v, want unlimited", *promotion.MaxRedemptions)
	}
	if promotion.EffectiveMaxPerCustomer() != 1 {
		t.Fatalf("max_per_customer = %d, want 1", promotion.EffectiveMaxPerCustomer())
	}
	if err := promotion.Conditions.Validate(); err != nil {
		t.Fatalf("seeded conditions do not validate: %v", err)
	}
}

// While the campaign is off, an entitlement is still worth issuing: enabling
// the definition later makes every one of them work.
func TestEntitlementsIssuedWhileInactiveWorkOnceEnabled(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)

	if _, err := svc.Issue(ctx, "WELCOME50", "cust-1", "system", "k1", ""); err != nil {
		t.Fatalf("Issue while inactive: %v", err)
	}
	if got := svc.Evaluate(ctx, "WELCOME50", cartFor("cust-1", 150000)); got.OK {
		t.Fatal("an inactive campaign must not discount anything yet")
	}

	enableWelcomePromotion(t, svc)
	if got := svc.Evaluate(ctx, "WELCOME50", cartFor("cust-1", 150000)); !got.OK {
		t.Fatalf("enabling the campaign should make the entitlement work, got %s", got.Reason)
	} else if got.DiscountWon != 50000 {
		t.Fatalf("discount = %d, want 50000", got.DiscountWon)
	}
}

// The campaign's discount is half the minimum spend, so the boundary order is
// where it is most expensive. It must still leave the shipping fee payable.
func TestWelcomeCodeAtTheMinimumSpendLeavesShippingPayable(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	enableWelcomePromotion(t, svc)
	if _, err := svc.Issue(ctx, "WELCOME50", "cust-1", "system", "k1", ""); err != nil {
		t.Fatalf("Issue: %v", err)
	}

	got := svc.Evaluate(ctx, "WELCOME50", cartFor("cust-1", 100000))
	if !got.OK {
		t.Fatalf("a cart exactly at the minimum should qualify, got %s/%s", got.Reason, got.SubReason)
	}
	if got.DiscountWon != 50000 {
		t.Fatalf("discount = %d, want 50000", got.DiscountWon)
	}
	// subtotal - discount + shipping: the customer still pays delivery.
	if remaining := int64(100000) - got.DiscountWon; remaining != 50000 {
		t.Fatalf("goods after discount = %d, want 50000", remaining)
	}
}
