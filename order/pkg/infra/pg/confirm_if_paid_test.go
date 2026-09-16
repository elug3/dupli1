package pg

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
)

func TestConfirmIfPaidUpdatesPaidOrder(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_confirm_if_paid_success_test")
	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := t.Context()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	order, err := domain.NewOrder("ord-confirm-pg-1", "cust-1", "res-confirm-1", []domain.OrderItem{{
		SkuID: "sku-1", SKU: "BAG-001", Quantity: 1, UnitPriceWon: 1000,
	}}, "", 0, 0, now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	if err := order.MarkPaid("pay-confirm-pg-1", order.TotalWon, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if err := repo.Save(ctx, order); err != nil {
		t.Fatalf("Save: %v", err)
	}

	confirmed := *order
	if err := confirmed.Confirm(now); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	ok, err := repo.ConfirmIfPaid(ctx, &confirmed, nil)
	if err != nil {
		t.Fatalf("ConfirmIfPaid: %v", err)
	}
	if !ok {
		t.Fatal("expected ConfirmIfPaid to persist confirmed")
	}

	got, err := repo.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != domain.StatusConfirmed {
		t.Fatalf("status = %q, want confirmed", got.Status)
	}
}

func TestConfirmIfPaidSkipsCanceledOrder(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_confirm_if_paid_canceled_test")
	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := t.Context()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	order, err := domain.NewOrder("ord-confirm-pg-2", "cust-1", "res-confirm-2", []domain.OrderItem{{
		SkuID: "sku-1", SKU: "BAG-001", Quantity: 1, UnitPriceWon: 1000,
	}}, "", 0, 0, now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	if err := order.MarkPaid("pay-confirm-pg-2", order.TotalWon, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if err := repo.Save(ctx, order); err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, ok, err := repo.CancelIfPaidForRefund(ctx, "ord-confirm-pg-2", "pay-confirm-pg-2", now, nil)
	if err != nil || !ok {
		t.Fatalf("CancelIfPaidForRefund: ok=%v err=%v", ok, err)
	}

	toConfirm := *order
	if err := toConfirm.Confirm(now); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	ok, err = repo.ConfirmIfPaid(ctx, &toConfirm, nil)
	if err != nil {
		t.Fatalf("ConfirmIfPaid: %v", err)
	}
	if ok {
		t.Fatal("expected ConfirmIfPaid to skip canceled order")
	}

	got, err := repo.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != domain.StatusCanceled {
		t.Fatalf("status = %q, want canceled", got.Status)
	}
}
