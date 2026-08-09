package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
)

func fixedNow() time.Time {
	return time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
}

func validOrder(t *testing.T) *domain.Order {
	t.Helper()
	order, err := domain.NewOrder("ord-1", "cust-1", "res-1", []domain.OrderItem{
		{SKU: "BAG-1", Quantity: 1, UnitPriceCents: 5000},
	}, "", 0, fixedNow())
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	return order
}

func TestOrderShipRejectsNonPaid(t *testing.T) {
	order := validOrder(t)
	if err := order.Ship("manager-1", fixedNow()); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("Ship pending order: err = %v, want ErrInvalidTransition", err)
	}
}

func TestOrderShipRequiresShippedBy(t *testing.T) {
	order := validOrder(t)
	if err := order.MarkPaid("pay-1", order.TotalCents, fixedNow()); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if err := order.Ship("  ", fixedNow()); !errors.Is(err, domain.ErrInvalidOrder) {
		t.Fatalf("Ship empty shipped_by: err = %v, want ErrInvalidOrder", err)
	}
}

func TestOrderReinstateForLatePayment(t *testing.T) {
	order := validOrder(t)
	now := fixedNow()
	if err := order.Cancel(now); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	later := now.Add(6 * time.Minute)
	if err := order.ReinstateForLatePayment("res-late", later); err != nil {
		t.Fatalf("ReinstateForLatePayment: %v", err)
	}
	if order.Status != domain.StatusPending {
		t.Fatalf("status = %q, want pending", order.Status)
	}
	if order.ReservationID != "res-late" {
		t.Fatalf("reservation_id = %q, want res-late", order.ReservationID)
	}
	if !order.PaymentDueAt.Equal(later.Add(domain.DefaultPaymentTTL)) {
		t.Fatalf("payment_due_at = %v, want %v", order.PaymentDueAt, later.Add(domain.DefaultPaymentTTL))
	}
}

func TestOrderReinstateForLatePayment_RejectsInvalidInput(t *testing.T) {
	order := validOrder(t)
	now := fixedNow()

	if err := order.ReinstateForLatePayment("res-late", now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("reinstate pending order: err = %v, want ErrInvalidTransition", err)
	}

	if err := order.Cancel(now); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if err := order.ReinstateForLatePayment("", now); !errors.Is(err, domain.ErrInvalidOrder) {
		t.Fatalf("reinstate empty reservation: err = %v, want ErrInvalidOrder", err)
	}
}

func TestOrderMarkPaidRejectsWrongAmount(t *testing.T) {
	order := validOrder(t)
	if err := order.MarkPaid("pay-1", order.TotalCents-1, fixedNow()); !errors.Is(err, domain.ErrPaymentAmountMismatch) {
		t.Fatalf("MarkPaid wrong amount: err = %v, want ErrPaymentAmountMismatch", err)
	}
}
