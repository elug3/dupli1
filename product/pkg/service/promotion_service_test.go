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
	return service.NewPromotionService(store).WithLedger(ledger), store
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

// Single-user codes need a Phase 3 entitlement; until then they are refused
// rather than quietly treated as public.
func TestSingleUserScopeIsRefusedUntilPhase3(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPromotionSvc(t)
	p := fixedPromotion("WELCOME1", 5000)
	p.Scope = domain.ScopeSingleUser
	if _, err := svc.Create(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := svc.Evaluate(ctx, "WELCOME1", cartFor("cust-1", 50000)); got.Reason != domain.ReasonLoginRequired {
		t.Fatalf("reason = %s, want login_required", got.Reason)
	}
}

var _ = errors.Is
