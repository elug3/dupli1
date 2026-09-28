package pg

import (
	"errors"
	"testing"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

func newPromotionStores(t *testing.T) (*PromotionStore, *PromotionRedemptionStore) {
	t.Helper()
	dsn := requireProductDSN(t)
	pool := freshInventorySchema(t, dsn, "promo_redemption_test")
	store := &PromotionStore{pool: pool}
	// NewPromotionStore runs renameCouponsTableIfNeeded against public.* while
	// these tests isolate tables in a dedicated search_path; migrate the fresh
	// schema directly instead.
	if err := store.migrateFreshSchema(); err != nil {
		t.Fatalf("migrate promotion schema: %v", err)
	}
	return store, NewPromotionRedemptionStore(pool)
}

func fixedPromotionForLedger(code string, maxRedemptions *int) domain.Promotion {
	return domain.Promotion{
		Code:           code,
		Scope:          domain.ScopeGlobal,
		Active:         true,
		MaxRedemptions: maxRedemptions,
		MaxPerCustomer: 1,
		Benefit: domain.Benefit{
			Target:           domain.BenefitTargetGoods,
			DiscountType:     domain.DiscountTypeFixed,
			DiscountFixedWon: 5000,
			ApplyTo:          domain.ApplyToEntireSubtotal,
		},
	}
}

func reserveInput(code, orderID, customerID string) ports.ReserveRedemptionInput {
	return ports.ReserveRedemptionInput{
		Code:                code,
		OrderID:             orderID,
		CustomerID:          customerID,
		DiscountWon:         5000,
		OrderSubtotalWon:    50000,
		EligibleSubtotalWon: 50000,
		AppliedBenefit: &domain.Benefit{
			Target:           domain.BenefitTargetGoods,
			DiscountType:     domain.DiscountTypeFixed,
			DiscountFixedWon: 5000,
			ApplyTo:          domain.ApplyToEntireSubtotal,
		},
	}
}

// Regression for PR #280: Reserve must lock the promotion row and refuse a
// second checkout once redemption_count reaches max_redemptions.
func TestPromotionRedemptionReserveEnforcesCampaignCapInPostgres(t *testing.T) {
	ctx := t.Context()
	promoStore, ledger := newPromotionStores(t)

	max := 1
	code := "ONE_SLOT_PG"
	if err := promoStore.Create(ctx, fixedPromotionForLedger(code, &max)); err != nil {
		t.Fatalf("create promotion: %v", err)
	}

	if _, err := ledger.Reserve(ctx, reserveInput(code, "ord-1", "cust-1"), 1); err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	if _, err := ledger.Reserve(ctx, reserveInput(code, "ord-2", "cust-2"), 1); err == nil {
		t.Fatal("second reserve should refuse when campaign cap is 1")
	} else if !ports.IsCampaignExhaustedConflict(err) {
		t.Fatalf("second reserve err = %v, want campaign exhausted conflict", err)
	}

	got, err := promoStore.Get(ctx, code)
	if err != nil {
		t.Fatalf("get promotion: %v", err)
	}
	if got.RedemptionCount != 1 {
		t.Fatalf("redemption_count = %d, want 1", got.RedemptionCount)
	}
}

func TestPromotionRedemptionReserveEnforcesPerCustomerLimitInPostgres(t *testing.T) {
	ctx := t.Context()
	promoStore, ledger := newPromotionStores(t)

	code := "ONCE_EACH"
	if err := promoStore.Create(ctx, fixedPromotionForLedger(code, nil)); err != nil {
		t.Fatalf("create promotion: %v", err)
	}

	if _, err := ledger.Reserve(ctx, reserveInput(code, "ord-1", "cust-1"), 1); err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	if _, err := ledger.Reserve(ctx, reserveInput(code, "ord-2", "cust-1"), 1); err == nil {
		t.Fatal("second reserve for same customer should be refused")
	} else if !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("second reserve err = %v, want conflict", err)
	}
}

func TestPromotionRedemptionReserveIsIdempotentForOrderInPostgres(t *testing.T) {
	ctx := t.Context()
	promoStore, ledger := newPromotionStores(t)

	code := "IDEMPOTENT"
	if err := promoStore.Create(ctx, fixedPromotionForLedger(code, nil)); err != nil {
		t.Fatalf("create promotion: %v", err)
	}

	in := reserveInput(code, "ord-retry", "cust-1")
	first, err := ledger.Reserve(ctx, in, 1)
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	second, err := ledger.Reserve(ctx, in, 1)
	if err != nil {
		t.Fatalf("retry reserve: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("retry returned redemption %q, want same row %q", second.ID, first.ID)
	}

	got, err := promoStore.Get(ctx, code)
	if err != nil {
		t.Fatalf("get promotion: %v", err)
	}
	if got.RedemptionCount != 1 {
		t.Fatalf("redemption_count = %d, want 1 after idempotent retry", got.RedemptionCount)
	}
}

func TestPromotionRedemptionReleaseFreesCampaignSlotInPostgres(t *testing.T) {
	ctx := t.Context()
	promoStore, ledger := newPromotionStores(t)

	max := 1
	code := "RELEASE_SLOT"
	if err := promoStore.Create(ctx, fixedPromotionForLedger(code, &max)); err != nil {
		t.Fatalf("create promotion: %v", err)
	}

	if _, err := ledger.Reserve(ctx, reserveInput(code, "ord-1", "cust-1"), 1); err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	if _, err := ledger.Reserve(ctx, reserveInput(code, "ord-2", "cust-2"), 1); !ports.IsCampaignExhaustedConflict(err) {
		t.Fatalf("campaign should be exhausted before release, err = %v", err)
	}

	now := time.Now().UTC()
	if err := ledger.Release(ctx, "ord-1", now); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := ledger.Reserve(ctx, reserveInput(code, "ord-2", "cust-2"), 1); err != nil {
		t.Fatalf("reserve after release: %v", err)
	}

	got, err := promoStore.Get(ctx, code)
	if err != nil {
		t.Fatalf("get promotion: %v", err)
	}
	if got.RedemptionCount != 1 {
		t.Fatalf("redemption_count = %d, want 1 after release + re-reserve", got.RedemptionCount)
	}
}

// The sign-up campaign used to be a compiled-in constant. An environment that
// already has its row is marked auto_issue once, when the column first
// appears, so sign-ups keep getting it; a manager clearing the flag afterwards
// must not see a later deploy put it back.
func TestAutoIssueMigrationCarriesTheSignUpCampaignOverOnce(t *testing.T) {
	store, _ := newPromotionStores(t)
	ctx := t.Context()

	// Rewind to the shape before auto_issue existed, holding the old campaign.
	if _, err := store.pool.Exec(ctx, `ALTER TABLE promotions DROP COLUMN auto_issue`); err != nil {
		t.Fatalf("drop auto_issue: %v", err)
	}
	if _, err := store.pool.Exec(ctx, `
		INSERT INTO promotions (code, scope, discount, active) VALUES
			('WELCOME50', 'single_user', 0, FALSE),
			('SUMMER30', 'global', 0.30, TRUE)
	`); err != nil {
		t.Fatalf("insert pre-migration rows: %v", err)
	}
	if err := store.migrateFreshSchema(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	welcome, err := store.Get(ctx, "WELCOME50")
	if err != nil {
		t.Fatalf("Get WELCOME50: %v", err)
	}
	if welcome.AutoIssue != domain.AutoIssueUserRegistered {
		t.Fatalf("WELCOME50 auto_issue = %q, want user_registered", welcome.AutoIssue)
	}
	summer, err := store.Get(ctx, "SUMMER30")
	if err != nil {
		t.Fatalf("Get SUMMER30: %v", err)
	}
	if summer.AutoIssue != domain.AutoIssueNone {
		t.Fatalf("SUMMER30 auto_issue = %q, want none", summer.AutoIssue)
	}

	none := domain.AutoIssueNone
	if _, err := store.Update(ctx, "WELCOME50", ports.PromotionPatch{AutoIssue: &none}); err != nil {
		t.Fatalf("clear auto_issue: %v", err)
	}
	if err := store.migrateFreshSchema(); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	welcome, err = store.Get(ctx, "WELCOME50")
	if err != nil {
		t.Fatalf("Get WELCOME50: %v", err)
	}
	if welcome.AutoIssue != domain.AutoIssueNone {
		t.Fatalf("a redeploy set auto_issue back to %q", welcome.AutoIssue)
	}
}

// Nothing is seeded any more: a new database starts with no codes.
func TestFreshPromotionSchemaHasNoCodes(t *testing.T) {
	store, _ := newPromotionStores(t)
	got, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("fresh schema lists %d codes, want 0", len(got))
	}
}
