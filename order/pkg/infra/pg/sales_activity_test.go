package pg

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
)

// The sales report reads payments by paid_at and refunds by canceled_at, so
// every cancel path has to stamp canceled_at and a refund outside the paid
// week still has to be found.
func TestListSalesActivityFindsPaymentsAndRefundsInRange(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_sales_activity_test")
	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := t.Context()

	paidAt := time.Date(2026, 9, 1, 3, 0, 0, 0, time.UTC)
	newPaid := func(id string) *domain.Order {
		o, err := domain.NewOrder(id, "cust-1", "res-"+id, []domain.OrderItem{{
			SkuID: "sku-1", SKU: "BAG-001", Quantity: 1, UnitPriceWon: 1000,
		}}, "", 0, 0, paidAt)
		if err != nil {
			t.Fatalf("NewOrder: %v", err)
		}
		if err := o.MarkPaid("pay-"+id, o.TotalWon, paidAt); err != nil {
			t.Fatalf("MarkPaid: %v", err)
		}
		if err := repo.Save(ctx, o); err != nil {
			t.Fatalf("Save: %v", err)
		}
		return o
	}
	newPaid("ord-live")
	newPaid("ord-refunded")

	refundedAt := time.Date(2026, 9, 20, 3, 0, 0, 0, time.UTC)
	canceled, ok, err := repo.CancelIfPaidForRefund(ctx, "ord-refunded", "pay-ord-refunded", refundedAt, nil)
	if err != nil || !ok {
		t.Fatalf("CancelIfPaidForRefund: ok=%v err=%v", ok, err)
	}
	if canceled.CanceledAt == nil || !canceled.CanceledAt.Equal(refundedAt) {
		t.Fatalf("CanceledAt = %v, want %v", canceled.CanceledAt, refundedAt)
	}

	// The week of the payment: both orders were paid in it.
	paidWeek, err := repo.ListSalesActivity(ctx, paidAt.Add(-time.Hour), paidAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("ListSalesActivity: %v", err)
	}
	if len(paidWeek) != 2 {
		t.Fatalf("paid-week rows = %d, want 2", len(paidWeek))
	}

	// The week of the refund: only the refunded order, with both stamps.
	refundWeek, err := repo.ListSalesActivity(ctx, refundedAt.Add(-time.Hour), refundedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("ListSalesActivity: %v", err)
	}
	if len(refundWeek) != 1 || refundWeek[0].ID != "ord-refunded" || refundWeek[0].PaidAt == nil ||
		refundWeek[0].CanceledAt == nil || refundWeek[0].TotalWon != 1000 {
		t.Fatalf("refund-week rows = %+v, want ord-refunded with paid_at and canceled_at", refundWeek)
	}
}

// Orders canceled before canceled_at existed take their cancel time from
// updated_at, which is final for a canceled order.
func TestMigrateBackfillsCanceledAt(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_canceled_at_backfill_test")
	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := t.Context()

	now := time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC)
	o, err := domain.NewOrder("ord-old-cancel", "cust-1", "res-1", []domain.OrderItem{{
		SkuID: "sku-1", SKU: "BAG-001", Quantity: 1, UnitPriceWon: 1000,
	}}, "", 0, 0, now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	if err := o.Cancel(now); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, o); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE orders SET canceled_at = NULL WHERE id = 'ord-old-cancel'`); err != nil {
		t.Fatal(err)
	}

	if err := repo.migrate(); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	loaded, err := repo.Get(ctx, "ord-old-cancel")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if loaded.CanceledAt == nil || !loaded.CanceledAt.Equal(now) {
		t.Fatalf("CanceledAt = %v, want backfilled %v", loaded.CanceledAt, now)
	}
}
