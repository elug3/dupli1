package pg

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
)

// The referral report reads the referrer, the rate and fulfilled_at back from
// Postgres; losing any of them on a save would quietly pay a referrer nothing.
func TestListReferralActivityRoundTripsTheReferral(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_referral_activity_test")
	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := t.Context()
	placed := time.Date(2026, 11, 2, 3, 0, 0, 0, time.UTC)

	newReferred := func(id string) *domain.Order {
		o, err := domain.NewOrder(id, "cust-1", "res-"+id, []domain.OrderItem{{
			SkuID: "sku-1", SKU: "BAG-001", Quantity: 1, UnitPriceWon: 100000,
		}}, "HUH15", 15000, 0, placed)
		if err != nil {
			t.Fatalf("NewOrder: %v", err)
		}
		if err := o.SetReferral("허경호", 500); err != nil {
			t.Fatalf("SetReferral: %v", err)
		}
		if err := o.MarkPaid("pay-"+id, o.TotalWon, placed); err != nil {
			t.Fatalf("MarkPaid: %v", err)
		}
		return o
	}

	done := newReferred("ord-done")
	done.Status = domain.StatusDelivered
	if err := done.Fulfill(time.Date(2026, 11, 20, 3, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Fulfill: %v", err)
	}
	if err := repo.Save(ctx, done); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.Save(ctx, newReferred("ord-open")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.Get(ctx, "ord-done")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ReferrerName != "허경호" || got.ReferralCommissionBps != 500 || got.FulfilledAt == nil {
		t.Fatalf("Get = %q/%d/%v, want the referral and fulfilled_at", got.ReferrerName, got.ReferralCommissionBps, got.FulfilledAt)
	}

	nov := time.Date(2026, 11, 1, 0, 0, 0, 0, domainKST)
	orders, err := repo.ListReferralActivity(ctx, nov, nov.AddDate(0, 1, 0))
	if err != nil {
		t.Fatalf("ListReferralActivity: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("orders = %d, want the fulfilled one and the open one", len(orders))
	}
	byCode, err := repo.ListReferralOrdersByCode(ctx, "HUH15")
	if err != nil || len(byCode) != 2 || byCode[0].ReferrerName != "허경호" || byCode[0].PaidAt == nil {
		t.Fatalf("ListReferralOrdersByCode = %+v, %v; want both paid orders", byCode, err)
	}
	if other, err := repo.ListReferralOrdersByCode(ctx, "OTHER"); err != nil || len(other) != 0 {
		t.Fatalf("OTHER = %+v, %v; want none", other, err)
	}
	oct := nov.AddDate(0, -1, 0)
	orders, err = repo.ListReferralActivity(ctx, oct, nov)
	if err != nil || len(orders) != 1 || orders[0].ID != "ord-open" {
		t.Fatalf("October = %+v, %v; want only the open order", orders, err)
	}
}

var domainKST = time.FixedZone("KST", 9*3600)
