package pg

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
)

// CancelIfPending atomically cancels only while status is still pending. This
// is the Postgres guard behind PR #282's payment-race fix.
func TestCancelIfPendingCancelsPendingOrderInPostgres(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_cancel_if_pending_success_test")
	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := t.Context()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	order, err := domain.NewOrder("ord-pend-pg-1", "cust-1", "res-1", []domain.OrderItem{{
		SkuID: "sku-1", SKU: "BAG-001", Quantity: 1, UnitPriceWon: 1000,
	}}, "", 0, 0, now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	if err := repo.Save(ctx, order); err != nil {
		t.Fatalf("Save: %v", err)
	}

	canceled, ok, err := repo.CancelIfPending(ctx, "ord-pend-pg-1", now, nil)
	if err != nil {
		t.Fatalf("CancelIfPending: %v", err)
	}
	if !ok || canceled == nil {
		t.Fatal("want canceled order")
	}
	if canceled.Status != domain.StatusCanceled {
		t.Fatalf("status = %q, want canceled", canceled.Status)
	}

	loaded, err := repo.Get(ctx, "ord-pend-pg-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if loaded.Status != domain.StatusCanceled {
		t.Fatalf("persisted status = %q, want canceled", loaded.Status)
	}
}

func TestCancelIfPendingSkipsPaidOrderInPostgres(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_cancel_if_pending_paid_test")
	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := t.Context()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	order, err := domain.NewOrder("ord-pend-pg-2", "cust-1", "res-2", []domain.OrderItem{{
		SkuID: "sku-2", SKU: "BAG-002", Quantity: 1, UnitPriceWon: 1000,
	}}, "", 0, 0, now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	if err := order.MarkPaid("pay-race-pg", order.TotalWon, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if err := repo.Save(ctx, order); err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, ok, err := repo.CancelIfPending(ctx, "ord-pend-pg-2", now, nil)
	if err != nil {
		t.Fatalf("CancelIfPending: %v", err)
	}
	if ok {
		t.Fatal("expected no cancel on paid order")
	}

	loaded, err := repo.Get(ctx, "ord-pend-pg-2")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if loaded.Status != domain.StatusPaid {
		t.Fatalf("status = %q, want paid", loaded.Status)
	}
	if loaded.PaymentID != "pay-race-pg" {
		t.Fatalf("payment_id = %q, want pay-race-pg", loaded.PaymentID)
	}
}

func TestCancelIfPendingIsIdempotentInPostgres(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_cancel_if_pending_idempotent_test")
	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := t.Context()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	order, err := domain.NewOrder("ord-pend-pg-3", "cust-1", "res-3", []domain.OrderItem{{
		SkuID: "sku-3", SKU: "BAG-003", Quantity: 1, UnitPriceWon: 1000,
	}}, "", 0, 0, now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	if err := repo.Save(ctx, order); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, ok, err := repo.CancelIfPending(ctx, "ord-pend-pg-3", now, nil); err != nil || !ok {
		t.Fatalf("first cancel: ok=%v err=%v", ok, err)
	}
	_, ok, err := repo.CancelIfPending(ctx, "ord-pend-pg-3", now, nil)
	if err != nil {
		t.Fatalf("second cancel: %v", err)
	}
	if ok {
		t.Fatal("replay must not cancel again")
	}
}
