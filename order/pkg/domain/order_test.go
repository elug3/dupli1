package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
)

func testOrder(t *testing.T) *domain.Order {
	t.Helper()
	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	order, err := domain.NewOrder("ord-1", "cust-1", "res-1", []domain.OrderItem{
		{SKU: "BAG-1", Quantity: 1, UnitPriceCents: 5000},
	}, "", 0, now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	return order
}

func TestMarkPaid_SucceedsFromPending(t *testing.T) {
	order := testOrder(t)
	now := time.Date(2026, 8, 10, 10, 5, 0, 0, time.UTC)
	if err := order.MarkPaid("pay-1", order.TotalCents, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if order.Status != domain.StatusPaid || order.PaymentID != "pay-1" {
		t.Fatalf("order = %+v", order)
	}
}

func TestMarkPaid_RejectsCanceled(t *testing.T) {
	order := testOrder(t)
	now := time.Date(2026, 8, 10, 10, 5, 0, 0, time.UTC)
	if err := order.Cancel(now); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	err := order.MarkPaid("pay-1", order.TotalCents, now)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
}

func TestShip_RejectsPending(t *testing.T) {
	order := testOrder(t)
	now := time.Date(2026, 8, 10, 10, 5, 0, 0, time.UTC)
	err := order.Ship("manager-1", now)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
}

func TestShip_SucceedsFromPaid(t *testing.T) {
	order := testOrder(t)
	now := time.Date(2026, 8, 10, 10, 5, 0, 0, time.UTC)
	if err := order.MarkPaid("pay-1", order.TotalCents, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if err := order.Ship("manager-1", now); err != nil {
		t.Fatalf("Ship: %v", err)
	}
	if order.Status != domain.StatusInTransit || order.ShippedBy != "manager-1" {
		t.Fatalf("order = %+v", order)
	}
}

func TestReinstateForLatePayment_FromCanceled(t *testing.T) {
	order := testOrder(t)
	now := time.Date(2026, 8, 10, 10, 5, 0, 0, time.UTC)
	if err := order.Cancel(now); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	later := now.Add(10 * time.Minute)
	if err := order.ReinstateForLatePayment("res-late", later); err != nil {
		t.Fatalf("ReinstateForLatePayment: %v", err)
	}
	if order.Status != domain.StatusPending {
		t.Fatalf("status = %q, want pending", order.Status)
	}
	if order.ReservationID != "res-late" {
		t.Fatalf("reservation_id = %q, want res-late", order.ReservationID)
	}
	if !order.PaymentDueAt.After(later) {
		t.Fatalf("payment_due_at = %v, want after %v", order.PaymentDueAt, later)
	}
}

func TestReinstateForLatePayment_RejectsNonCanceled(t *testing.T) {
	order := testOrder(t)
	now := time.Date(2026, 8, 10, 10, 5, 0, 0, time.UTC)
	err := order.ReinstateForLatePayment("res-late", now)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
}
