package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
)

func newTestOrder(t *testing.T) *domain.Order {
	t.Helper()
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	order, err := domain.NewOrder("ord-1", "customer-1", "res-1", []domain.OrderItem{
		{SKU: "BAG-1", Quantity: 1, UnitPriceCents: 70000},
	}, "", 0, now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	return order
}

func TestNewOrderRejectsInvalidInput(t *testing.T) {
	now := time.Now()
	_, err := domain.NewOrder("", "customer-1", "res-1", []domain.OrderItem{
		{SKU: "BAG-1", Quantity: 1, UnitPriceCents: 1000},
	}, "", 0, now)
	if !errors.Is(err, domain.ErrInvalidOrder) {
		t.Fatalf("empty id err = %v, want ErrInvalidOrder", err)
	}

	_, err = domain.NewOrder("ord-1", "customer-1", "res-1", nil, "", 0, now)
	if !errors.Is(err, domain.ErrInvalidOrder) {
		t.Fatalf("empty items err = %v, want ErrInvalidOrder", err)
	}

	_, err = domain.NewOrder("ord-1", "customer-1", "res-1", []domain.OrderItem{
		{SKU: "BAG-1", Quantity: 0, UnitPriceCents: 1000},
	}, "", 0, now)
	if !errors.Is(err, domain.ErrInvalidOrder) {
		t.Fatalf("zero quantity err = %v, want ErrInvalidOrder", err)
	}
}

func TestMarkPaidRequiresPendingAndMatchingAmount(t *testing.T) {
	order := newTestOrder(t)
	now := time.Date(2026, 8, 11, 10, 5, 0, 0, time.UTC)

	if err := order.MarkPaid("pay-1", order.TotalCents, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if order.Status != domain.StatusPaid || order.PaymentID != "pay-1" || order.PaidAt == nil {
		t.Fatalf("order = %+v, want paid with payment id", order)
	}

	if err := order.MarkPaid("pay-2", order.TotalCents, now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("second MarkPaid err = %v, want ErrInvalidTransition", err)
	}

	pending := newTestOrder(t)
	if err := pending.MarkPaid("", pending.TotalCents, now); !errors.Is(err, domain.ErrInvalidOrder) {
		t.Fatalf("empty payment id err = %v, want ErrInvalidOrder", err)
	}
	if err := pending.MarkPaid("pay-1", pending.TotalCents-1, now); !errors.Is(err, domain.ErrPaymentAmountMismatch) {
		t.Fatalf("amount mismatch err = %v, want ErrPaymentAmountMismatch", err)
	}
}

func TestShipRequiresPaidOrder(t *testing.T) {
	order := newTestOrder(t)
	now := time.Date(2026, 8, 11, 10, 10, 0, 0, time.UTC)
	tracking := domain.ShipmentTracking{Carrier: domain.CarrierCJ, TrackingNumber: "1234567890"}

	if err := order.Ship("manager-1", tracking, now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("ship pending err = %v, want ErrInvalidTransition", err)
	}

	if err := order.MarkPaid("pay-1", order.TotalCents, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if err := order.Ship("", tracking, now); !errors.Is(err, domain.ErrInvalidOrder) {
		t.Fatalf("empty shippedBy err = %v, want ErrInvalidOrder", err)
	}
	if err := order.Ship("manager-1", domain.ShipmentTracking{}, now); !errors.Is(err, domain.ErrInvalidShipment) {
		t.Fatalf("empty tracking err = %v, want ErrInvalidShipment", err)
	}
	if err := order.Ship("manager-1", tracking, now); err != nil {
		t.Fatalf("Ship: %v", err)
	}
	if order.Status != domain.StatusInTransit || order.ShippedBy != "manager-1" || order.ShippedAt == nil {
		t.Fatalf("order = %+v, want in_transit with ship metadata", order)
	}
	if order.Carrier != domain.CarrierCJ || order.TrackingNumber != "1234567890" {
		t.Fatalf("tracking = %s/%s", order.Carrier, order.TrackingNumber)
	}
}

func TestReinstateForLatePayment(t *testing.T) {
	order := newTestOrder(t)
	now := time.Date(2026, 8, 11, 10, 15, 0, 0, time.UTC)

	if err := order.Cancel(now); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if err := order.ReinstateForLatePayment("res-late", now); err != nil {
		t.Fatalf("ReinstateForLatePayment: %v", err)
	}
	if order.Status != domain.StatusPending {
		t.Fatalf("status = %q, want pending", order.Status)
	}
	if order.ReservationID != "res-late" {
		t.Fatalf("reservation_id = %q, want res-late", order.ReservationID)
	}
	if !order.PaymentDueAt.After(now) {
		t.Fatalf("payment_due_at = %v, want after reinstate time", order.PaymentDueAt)
	}

	paid := newTestOrder(t)
	if err := paid.MarkPaid("pay-1", paid.TotalCents, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if err := paid.ReinstateForLatePayment("res-x", now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("reinstate paid err = %v, want ErrInvalidTransition", err)
	}
}

func TestCancelAndFulfillTransitions(t *testing.T) {
	order := newTestOrder(t)
	now := time.Date(2026, 8, 11, 10, 20, 0, 0, time.UTC)

	if err := order.Cancel(now); err != nil {
		t.Fatalf("Cancel pending: %v", err)
	}
	if order.Status != domain.StatusCanceled {
		t.Fatalf("status = %q, want canceled", order.Status)
	}
	if err := order.Cancel(now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("cancel canceled err = %v, want ErrInvalidTransition", err)
	}

	paid := newTestOrder(t)
	if err := paid.MarkPaid("pay-1", paid.TotalCents, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if err := paid.Ship("manager-1", domain.ShipmentTracking{Carrier: domain.CarrierHanjin, TrackingNumber: "HN-1"}, now); err != nil {
		t.Fatalf("Ship: %v", err)
	}
	if err := paid.Fulfill(now); err != nil {
		t.Fatalf("Fulfill: %v", err)
	}
	if paid.Status != domain.StatusFulfilled {
		t.Fatalf("status = %q, want fulfilled", paid.Status)
	}
}

func TestIsPaymentExpired(t *testing.T) {
	order := newTestOrder(t)
	beforeDue := order.PaymentDueAt.Add(-time.Minute)
	afterDue := order.PaymentDueAt.Add(time.Minute)

	if order.IsPaymentExpired(beforeDue) {
		t.Fatal("expected pending order before due not expired")
	}
	if !order.IsPaymentExpired(afterDue) {
		t.Fatal("expected pending order after due to be expired")
	}

	if err := order.MarkPaid("pay-1", order.TotalCents, afterDue); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if order.IsPaymentExpired(afterDue) {
		t.Fatal("paid order must not report payment expired")
	}
}
