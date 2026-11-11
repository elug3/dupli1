package service_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/infra/memory"
	"github.com/elug3/dupli1/order/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/events"
)

// statusEvents decodes every order.status_updated the service published.
func statusEvents(t *testing.T, pub *recordedPublisher) []events.Order {
	t.Helper()
	var out []events.Order
	for i, subject := range pub.subjects {
		if subject != events.OrderStatusUpdate {
			continue
		}
		raw, err := json.Marshal(pub.events[i])
		if err != nil {
			t.Fatalf("marshal event: %v", err)
		}
		var e events.Order
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		out = append(out, e)
	}
	return out
}

func lastStatusEvent(t *testing.T, pub *recordedPublisher) events.Order {
	t.Helper()
	all := statusEvents(t, pub)
	if len(all) == 0 {
		t.Fatal("no order.status_updated published")
	}
	return all[len(all)-1]
}

func wantChange(t *testing.T, got events.Order, from, to, action string, automatic bool) {
	t.Helper()
	if got.PreviousStatus != from || got.Status != to || got.Change != action || got.Automatic != automatic {
		t.Fatalf("event = %s %s→%s automatic=%v, want %s %s→%s automatic=%v",
			got.Change, got.PreviousStatus, got.Status, got.Automatic, action, from, to, automatic)
	}
}

// Every status event says what it changed, so the ops alert can show the
// status the order left as well as the one it is in.
func TestStatusEventsCarryPreviousStatusAndAction(t *testing.T) {
	ctx := t.Context()
	pub := &recordedPublisher{}
	svc := newSvc(&fakeStock{reservationID: "res-1"}, &fakeProduct{defaultKRW: 5000}, pub).
		WithPayment(&fakePayment{})

	order, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if _, err := svc.MarkOrderPaid(ctx, order.ID, "pay-1", order.TotalWon); err != nil {
		t.Fatalf("MarkOrderPaid: %v", err)
	}
	wantChange(t, lastStatusEvent(t, pub), "pending", "paid", events.OrderChangePaid, false)

	if _, err := svc.ConfirmOrder(ctx, order.ID); err != nil {
		t.Fatalf("ConfirmOrder: %v", err)
	}
	wantChange(t, lastStatusEvent(t, pub), "paid", "confirmed", events.OrderChangeConfirmed, false)

	if _, err := svc.CustomerCancel(ctx, order.ID, "wrong size"); err != nil {
		t.Fatalf("CustomerCancel: %v", err)
	}
	requested := lastStatusEvent(t, pub)
	wantChange(t, requested, "confirmed", "confirmed", events.OrderChangeCancelRequested, false)
	if requested.Reason != "wrong size" {
		t.Fatalf("reason = %q, want the shopper's", requested.Reason)
	}

	if _, err := svc.RejectCancelRequest(ctx, order.ID); err != nil {
		t.Fatalf("RejectCancelRequest: %v", err)
	}
	wantChange(t, lastStatusEvent(t, pub), "confirmed", "confirmed", events.OrderChangeCancelRejected, false)

	if _, err := svc.CancelOrder(ctx, order.ID); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	canceled := lastStatusEvent(t, pub)
	wantChange(t, canceled, "confirmed", "canceled", events.OrderChangeCanceled, false)
	if canceled.Reason != "" {
		t.Fatalf("a manager cancel carries no shopper reason, got %q", canceled.Reason)
	}
}

func TestShipEventCarriesTracking(t *testing.T) {
	ctx := t.Context()
	pub := &recordedPublisher{}
	stock := &fakeStock{reservationID: "res-1"}
	svc := newSvc(stock, &fakeProduct{defaultKRW: 5000}, pub).
		WithInspectionVideos(memory.NewVideoStore(), 1000)
	order := confirmedOrder(t, svc, "c-1")
	if _, err := upload(svc, order.ID, "video/mp4", "film"); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if _, err := svc.ShipOrder(ctx, order.ID, "staff-1", testShipTracking()); err != nil {
		t.Fatalf("ShipOrder: %v", err)
	}
	shipped := lastStatusEvent(t, pub)
	wantChange(t, shipped, "confirmed", "in_transit", events.OrderChangeShipped, false)
	if shipped.Carrier != domain.CarrierCJ || shipped.TrackingNumber != "123456789012" {
		t.Fatalf("tracking = %q %q", shipped.Carrier, shipped.TrackingNumber)
	}
}

// The policy workers' changes are marked automatic, so ops can tell the
// 2-hour auto-confirm and the auto-approved cancel from a colleague's.
func TestPolicyWorkerChangesAreAutomatic(t *testing.T) {
	ctx := t.Context()
	clock := time.Date(2026, 9, 11, 8, 0, 0, 0, time.UTC)
	pub := &recordedPublisher{}
	svc := newSvc(&fakeStock{reservationID: "res-1"}, &fakeProduct{defaultKRW: 5000}, pub).
		WithPayment(&fakePayment{}).
		WithClock(func() time.Time { return clock })

	paid, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if _, err := svc.MarkOrderPaid(ctx, paid.ID, "pay-1", paid.TotalWon); err != nil {
		t.Fatalf("MarkOrderPaid: %v", err)
	}
	requested, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-2",
		Items:      []domain.OrderItem{{SKU: "bag-2", Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("CreateOrder 2: %v", err)
	}
	if _, err := svc.MarkOrderPaid(ctx, requested.ID, "pay-2", requested.TotalWon); err != nil {
		t.Fatalf("MarkOrderPaid 2: %v", err)
	}
	if _, err := svc.ConfirmOrder(ctx, requested.ID); err != nil {
		t.Fatalf("ConfirmOrder: %v", err)
	}
	if _, err := svc.CustomerCancel(ctx, requested.ID, "changed mind"); err != nil {
		t.Fatalf("CustomerCancel: %v", err)
	}

	clock = clock.Add(3 * time.Hour)
	before := len(statusEvents(t, pub))
	if err := svc.EnforceRefundPolicy(ctx); err != nil {
		t.Fatalf("EnforceRefundPolicy: %v", err)
	}
	got := statusEvents(t, pub)[before:]
	byOrder := map[string]events.Order{}
	for _, e := range got {
		byOrder[e.OrderID] = e
	}
	wantChange(t, byOrder[paid.ID], "paid", "confirmed", events.OrderChangeConfirmed, true)
	wantChange(t, byOrder[requested.ID], "confirmed", "canceled", events.OrderChangeCancelApproved, true)
}

// Order events carry the payment method, so ops alerts can mark a bypass
// order instead of presenting it as a card sale.
func TestOrderEventsCarryPaymentMethod(t *testing.T) {
	ctx := t.Context()
	pub := &recordedPublisher{}
	svc := newSvc(&fakeStock{reservationID: "res-1"}, &fakeProduct{defaultKRW: 5000}, pub)

	order, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID:    "customer-1",
		Items:         []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
		PaymentMethod: domain.PaymentMethodBypass,
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if _, err := svc.MarkOrderPaid(ctx, order.ID, "pay-1", order.TotalWon); err != nil {
		t.Fatalf("MarkOrderPaid: %v", err)
	}
	if len(pub.events) < 3 {
		t.Fatalf("published %v, want created, paid and status_updated", pub.subjects)
	}
	for i, ev := range pub.events {
		raw, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		var e events.Order
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Fatal(err)
		}
		if e.PaymentMethod != events.PaymentMethodBypass {
			t.Fatalf("%s payment_method = %q, want bypass", pub.subjects[i], e.PaymentMethod)
		}
	}
}
