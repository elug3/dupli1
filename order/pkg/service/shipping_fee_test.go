package service_test

import (
	"encoding/json"
	"testing"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/infra/memory"
	"github.com/elug3/dupli1/order/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/events"
)

func TestCreateOrder_AppliesConfiguredShippingFee(t *testing.T) {
	ctx := t.Context()
	svc := service.New(memory.NewRepository(), &fakeStock{}).
		WithProduct(&fakeProduct{defaultKRW: 250000}).
		WithShippingFee(3000)

	order, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if order.ShippingFeeWon != 3000 {
		t.Fatalf("shipping = %d, want the configured 3000", order.ShippingFeeWon)
	}
	if order.TotalWon != 253000 {
		t.Fatalf("total = %d, want 253000", order.TotalWon)
	}
}

// A Service built without WithShippingFee must not invent a delivery charge.
// This is the zero value of the service layer, not the deployment default —
// bootstrap always passes the configured fee (30,000 KRW unless overridden).
func TestCreateOrder_UnconfiguredServiceChargesNothing(t *testing.T) {
	ctx := t.Context()
	svc := service.New(memory.NewRepository(), &fakeStock{}).
		WithProduct(&fakeProduct{defaultKRW: 250000})

	order, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if order.ShippingFeeWon != 0 || order.TotalWon != 250000 {
		t.Fatalf("shipping = %d, total = %d; want 0 / 250000", order.ShippingFeeWon, order.TotalWon)
	}
}

// A misconfigured negative fee must be ignored rather than making orders
// cheaper than their goods.
func TestWithShippingFee_IgnoresNegative(t *testing.T) {
	ctx := t.Context()
	svc := service.New(memory.NewRepository(), &fakeStock{}).
		WithProduct(&fakeProduct{defaultKRW: 250000}).
		WithShippingFee(-500)

	order, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if order.ShippingFeeWon != 0 {
		t.Fatalf("shipping = %d, want a negative fee ignored", order.ShippingFeeWon)
	}
	if order.TotalWon != 250000 {
		t.Fatalf("total = %d, want 250000", order.TotalWon)
	}
}

// The fee is snapshotted on the order, so re-reading it after the configured
// fee changes must still show what the customer agreed to pay.
func TestCreateOrder_ShippingFeeIsSnapshotted(t *testing.T) {
	ctx := t.Context()
	repo := memory.NewRepository()
	svc := service.New(repo, &fakeStock{}).
		WithProduct(&fakeProduct{defaultKRW: 250000}).
		WithShippingFee(3000)

	order, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	svc.WithShippingFee(9000) // fee raised after the order was placed

	reloaded, err := svc.GetOrder(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if reloaded.ShippingFeeWon != 3000 || reloaded.TotalWon != 253000 {
		t.Fatalf("reloaded shipping = %d total = %d; a config change must not re-price a placed order",
			reloaded.ShippingFeeWon, reloaded.TotalWon)
	}
}

// Notification and any future subscriber need the fee on the wire to reconcile
// the total they are shown.
func TestCreateOrder_PublishesShippingFeeInEvent(t *testing.T) {
	ctx := t.Context()
	publisher := &recordedPublisher{}
	svc := service.New(memory.NewRepository(), &fakeStock{}, publisher).
		WithProduct(&fakeProduct{defaultKRW: 250000}).
		WithShippingFee(3000)

	if _, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
	}); err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if len(publisher.events) == 0 {
		t.Fatal("no event published")
	}
	raw, err := json.Marshal(publisher.events[0])
	if err != nil {
		t.Fatal(err)
	}
	var ev events.Order
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.ShippingFeeWon != 3000 {
		t.Fatalf("event shipping = %d, want 3000", ev.ShippingFeeWon)
	}
	if ev.SubtotalWon+ev.ShippingFeeWon-ev.DiscountWon != ev.TotalWon {
		t.Fatalf("event totals do not reconcile: %+v", ev)
	}
}

// Completing a session must charge the fee quoted when it opened, even if the
// configured amount changed mid-checkout.
func TestCompleteCheckout_UsesSessionQuotedShippingFee(t *testing.T) {
	ctx := t.Context()
	repo := memory.NewRepository()
	svc := service.NewWithCheckout(repo, &fakeStock{reservationID: "res-fee"}, nil, 0).
		WithProduct(&fakeProduct{defaultKRW: 250000}).
		WithShippingFee(3000)

	session, err := svc.CreateCheckoutSession(ctx, service.CreateCheckoutSessionInput{CustomerID: "customer-1"})
	if err != nil {
		t.Fatalf("CreateCheckoutSession: %v", err)
	}
	if _, err := svc.UpsertCheckoutItem(ctx, session.ID, domain.OrderItem{SKU: "bag-1", Quantity: 1}); err != nil {
		t.Fatalf("UpsertCheckoutItem: %v", err)
	}

	svc.WithShippingFee(9000)

	result, err := svc.CompleteCheckout(ctx, session.ID, testCompleteCheckoutInput())
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}
	if result.Order.ShippingFeeWon != 3000 {
		t.Fatalf("order shipping = %d, want the session-quoted 3000", result.Order.ShippingFeeWon)
	}
	if result.Order.TotalWon != 253000 {
		t.Fatalf("order total = %d, want 253000", result.Order.TotalWon)
	}
}
