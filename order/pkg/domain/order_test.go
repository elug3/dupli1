package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
)

func newTestOrder(t *testing.T) *domain.Order {
	t.Helper()
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	order, err := domain.NewOrder("ord-1", "customer-1", "res-1", []domain.OrderItem{
		{SKU: "BAG-1", Quantity: 1, UnitPriceKRW: 70000},
	}, "", 0, 0, now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	return order
}

func TestNewOrderRejectsInvalidInput(t *testing.T) {
	now := time.Now()
	_, err := domain.NewOrder("", "customer-1", "res-1", []domain.OrderItem{
		{SKU: "BAG-1", Quantity: 1, UnitPriceKRW: 1000},
	}, "", 0, 0, now)
	if !errors.Is(err, domain.ErrInvalidOrder) {
		t.Fatalf("empty id err = %v, want ErrInvalidOrder", err)
	}

	_, err = domain.NewOrder("ord-1", "customer-1", "res-1", nil, "", 0, 0, now)
	if !errors.Is(err, domain.ErrInvalidOrder) {
		t.Fatalf("empty items err = %v, want ErrInvalidOrder", err)
	}

	_, err = domain.NewOrder("ord-1", "customer-1", "res-1", []domain.OrderItem{
		{SKU: "BAG-1", Quantity: 0, UnitPriceKRW: 1000},
	}, "", 0, 0, now)
	if !errors.Is(err, domain.ErrInvalidOrder) {
		t.Fatalf("zero quantity err = %v, want ErrInvalidOrder", err)
	}
}

func TestMarkPaidRequiresPendingAndMatchingAmount(t *testing.T) {
	order := newTestOrder(t)
	now := time.Date(2026, 8, 11, 10, 5, 0, 0, time.UTC)

	if err := order.MarkPaid("pay-1", order.TotalKRW, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if order.Status != domain.StatusPaid || order.PaymentID != "pay-1" || order.PaidAt == nil {
		t.Fatalf("order = %+v, want paid with payment id", order)
	}

	if err := order.MarkPaid("pay-2", order.TotalKRW, now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("second MarkPaid err = %v, want ErrInvalidTransition", err)
	}

	pending := newTestOrder(t)
	if err := pending.MarkPaid("", pending.TotalKRW, now); !errors.Is(err, domain.ErrInvalidOrder) {
		t.Fatalf("empty payment id err = %v, want ErrInvalidOrder", err)
	}
	if err := pending.MarkPaid("pay-1", pending.TotalKRW-1, now); !errors.Is(err, domain.ErrPaymentAmountMismatch) {
		t.Fatalf("amount mismatch err = %v, want ErrPaymentAmountMismatch", err)
	}
}

func TestShipRequiresConfirmedOrder(t *testing.T) {
	order := newTestOrder(t)
	now := time.Date(2026, 8, 11, 10, 10, 0, 0, time.UTC)
	tracking := domain.ShipmentTracking{Carrier: domain.CarrierCJ, TrackingNumber: "1234567890"}

	if err := order.Ship("manager-1", tracking, now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("ship pending err = %v, want ErrInvalidTransition", err)
	}

	if err := order.MarkPaid("pay-1", order.TotalKRW, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if err := order.Ship("manager-1", tracking, now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("ship unconfirmed paid err = %v, want ErrInvalidTransition", err)
	}
	if err := order.Confirm(now); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if order.Status != domain.StatusConfirmed || order.ConfirmedAt == nil {
		t.Fatalf("order = %+v, want confirmed with confirmed_at set", order)
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

	// Shipping again (already in_transit) is not a valid transition.
	if err := order.Ship("manager-1", tracking, now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("re-ship err = %v, want ErrInvalidTransition", err)
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
	if err := paid.MarkPaid("pay-1", paid.TotalKRW, now); err != nil {
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
	if err := paid.MarkPaid("pay-1", paid.TotalKRW, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if err := paid.Fulfill(now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("fulfill before delivered err = %v, want ErrInvalidTransition", err)
	}
	if err := paid.Confirm(now); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if err := paid.Ship("manager-1", domain.ShipmentTracking{Carrier: domain.CarrierHanjin, TrackingNumber: "HN-1"}, now); err != nil {
		t.Fatalf("Ship: %v", err)
	}
	if err := paid.Deliver("driver-1", now); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if paid.Status != domain.StatusDelivered || paid.DeliveredBy != "driver-1" || paid.DeliveredAt == nil {
		t.Fatalf("order = %+v, want delivered with delivery metadata", paid)
	}
	if err := paid.Fulfill(now); err != nil {
		t.Fatalf("Fulfill: %v", err)
	}
	if paid.Status != domain.StatusFulfilled {
		t.Fatalf("status = %q, want fulfilled", paid.Status)
	}
	if err := paid.Cancel(now); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("cancel fulfilled err = %v, want ErrInvalidTransition", err)
	}

	inTransit := newTestOrder(t)
	if err := inTransit.MarkPaid("pay-2", inTransit.TotalKRW, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if err := inTransit.Confirm(now); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if err := inTransit.Ship("manager-1", domain.ShipmentTracking{Carrier: domain.CarrierHanjin, TrackingNumber: "HN-2"}, now); err != nil {
		t.Fatalf("Ship: %v", err)
	}
	if err := inTransit.Cancel(now); err != nil {
		t.Fatalf("Cancel in_transit: %v", err)
	}
	if inTransit.Status != domain.StatusCanceled {
		t.Fatalf("status = %q, want canceled", inTransit.Status)
	}
}

func TestDeliveryReceiptAndDispute(t *testing.T) {
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)

	shipped := func() *domain.Order {
		o := newTestOrder(t)
		if err := o.MarkPaid("pay-1", o.TotalKRW, now); err != nil {
			t.Fatalf("MarkPaid: %v", err)
		}
		if err := o.Confirm(now); err != nil {
			t.Fatalf("Confirm: %v", err)
		}
		if err := o.Ship("manager-1", domain.ShipmentTracking{Carrier: domain.CarrierHanjin, TrackingNumber: "HN-1"}, now); err != nil {
			t.Fatalf("Ship: %v", err)
		}
		return o
	}

	t.Run("confirm receipt", func(t *testing.T) {
		o := shipped()
		if err := o.ConfirmReceipt(now); !errors.Is(err, domain.ErrInvalidTransition) {
			t.Fatalf("confirm receipt before delivered err = %v, want ErrInvalidTransition", err)
		}
		if err := o.Deliver("driver-1", now); err != nil {
			t.Fatalf("Deliver: %v", err)
		}
		later := now.Add(time.Hour)
		if err := o.ConfirmReceipt(later); err != nil {
			t.Fatalf("ConfirmReceipt: %v", err)
		}
		if o.Status != domain.StatusFulfilled || o.ReceiptConfirmedAt == nil || !o.ReceiptConfirmedAt.Equal(later) {
			t.Fatalf("order = %+v, want fulfilled with receipt_confirmed_at set", o)
		}
	})

	t.Run("report not received opens a dispute a manager resolves", func(t *testing.T) {
		o := shipped()
		if err := o.Deliver("driver-1", now); err != nil {
			t.Fatalf("Deliver: %v", err)
		}
		if err := o.ReportNotReceived("never arrived", now.Add(time.Hour)); err != nil {
			t.Fatalf("ReportNotReceived: %v", err)
		}
		if o.Status != domain.StatusDisputed || o.DisputedAt == nil || o.DisputeReason != "never arrived" {
			t.Fatalf("order = %+v, want disputed with reason", o)
		}
		// Once disputed, it is no longer a plain cancel-request candidate...
		if o.AllowsCancelRequest() {
			t.Fatal("disputed order must not offer a fresh cancel request")
		}
		// ...but a manager can still resolve it either way: fulfilled (proof
		// of delivery) or canceled (refund).
		if err := o.ResolveDisputeFulfilled(now.Add(2 * time.Hour)); err != nil {
			t.Fatalf("ResolveDisputeFulfilled: %v", err)
		}
		if o.Status != domain.StatusFulfilled {
			t.Fatalf("status = %q, want fulfilled", o.Status)
		}
	})

	t.Run("manager can cancel a dispute instead, and it refunds without restocking", func(t *testing.T) {
		o := shipped()
		if err := o.Deliver("driver-1", now); err != nil {
			t.Fatalf("Deliver: %v", err)
		}
		if err := o.ReportNotReceived("", now); err != nil {
			t.Fatalf("ReportNotReceived: %v", err)
		}
		if !o.StockCommitted() {
			t.Fatal("disputed order must still report stock committed")
		}
		if err := o.Cancel(now.Add(time.Hour)); err != nil {
			t.Fatalf("Cancel disputed: %v", err)
		}
		if o.Status != domain.StatusCanceled {
			t.Fatalf("status = %q, want canceled", o.Status)
		}
	})

	t.Run("cancel request flows through delivered", func(t *testing.T) {
		o := shipped()
		if err := o.Deliver("driver-1", now); err != nil {
			t.Fatalf("Deliver: %v", err)
		}
		if !o.AllowsCancelRequest() {
			t.Fatal("delivered order must still allow a cancel request")
		}
		if err := o.RequestCancel("wrong size", now); err != nil {
			t.Fatalf("RequestCancel: %v", err)
		}
		if o.CancelConfirmDueAtTime() == nil {
			t.Fatal("delivered cancel request must carry a manager response due time")
		}
	})

	t.Run("auto-fulfill window", func(t *testing.T) {
		o := shipped()
		if err := o.Deliver("driver-1", now); err != nil {
			t.Fatalf("Deliver: %v", err)
		}
		justBefore := now.Add(domain.DeliveryAutoFulfillWindow - time.Minute)
		if o.AutoFulfillOverdueAt(justBefore) {
			t.Fatal("must not be overdue before the 14-day window closes")
		}
		atWindow := now.Add(domain.DeliveryAutoFulfillWindow)
		if !o.AutoFulfillOverdueAt(atWindow) {
			t.Fatal("must be overdue once the 14-day window closes")
		}
	})
}

func TestRefundPolicyImmediateVsRequest(t *testing.T) {
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	pending := newTestOrder(t)
	if !pending.AllowsImmediateCancel() || pending.AllowsCancelRequest() {
		t.Fatal("pending must allow immediate cancel only")
	}

	paid := newTestOrder(t)
	if err := paid.MarkPaid("pay-1", paid.TotalKRW, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if !paid.AllowsImmediateCancel() || paid.ConfirmedAt != nil {
		t.Fatal("unconfirmed paid must allow immediate cancel")
	}
	if err := paid.Confirm(now); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if paid.AllowsImmediateCancel() || !paid.AllowsCancelRequest() {
		t.Fatal("confirmed paid must require a cancel request")
	}

	later := now.Add(domain.ManagerConfirmationWindow)
	paid.ApplyRefundPolicy(later)
	if paid.ConfirmationOverdue {
		t.Fatal("confirmed order must not report confirmation overdue")
	}

	unconfirmed := newTestOrder(t)
	if err := unconfirmed.MarkPaid("pay-2", unconfirmed.TotalKRW, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	unconfirmed.ApplyRefundPolicy(now.Add(domain.ManagerConfirmationWindow))
	if !unconfirmed.ConfirmationOverdue || unconfirmed.ConfirmationDueAt == nil {
		t.Fatal("unconfirmed paid order is overdue after 2 hours")
	}

	if err := paid.RequestCancel("changed mind", now); err != nil {
		t.Fatalf("RequestCancel: %v", err)
	}
	if paid.AllowsCancelRequest() {
		t.Fatal("second cancel request must not be allowed")
	}
	if err := paid.RequestCancel("again", now.Add(time.Minute)); err != nil {
		t.Fatalf("idempotent RequestCancel: %v", err)
	}
	paid.ApplyRefundPolicy(now.Add(domain.ManagerConfirmationWindow))
	if !paid.CancelConfirmOverdue {
		t.Fatal("cancel request is overdue after 2 hours")
	}
	if err := paid.RejectCancelRequest(now.Add(time.Hour)); err != nil {
		t.Fatalf("RejectCancelRequest: %v", err)
	}
	if paid.CancelRequestedAt != nil || paid.CancelRequestReason != "" {
		t.Fatal("reject must clear the cancel request")
	}
}

func TestTrimCancelReasonTruncatesLongNotes(t *testing.T) {
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	paid := newTestOrder(t)
	if err := paid.MarkPaid("pay-1", paid.TotalKRW, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if err := paid.Confirm(now); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	long := strings.Repeat("x", domain.MaxCancelRequestReasonLen+50)
	if err := paid.RequestCancel(long, now); err != nil {
		t.Fatalf("RequestCancel: %v", err)
	}
	if len(paid.CancelRequestReason) != domain.MaxCancelRequestReasonLen {
		t.Fatalf("reason len = %d, want %d", len(paid.CancelRequestReason), domain.MaxCancelRequestReasonLen)
	}
	if paid.CancelRequestReason != strings.Repeat("x", domain.MaxCancelRequestReasonLen) {
		t.Fatal("reason must be truncated without altering prefix")
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

	if err := order.MarkPaid("pay-1", order.TotalKRW, afterDue); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if order.IsPaymentExpired(afterDue) {
		t.Fatal("paid order must not report payment expired")
	}
}
