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
