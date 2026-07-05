package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/infra/memory"
	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/elug3/dupli1/order/pkg/service"
)

type fakeInventory struct {
	reservedItems []ports.InventoryItem
	reservationID string
	committed     string
	released      string
}

func (f *fakeInventory) Reserve(ctx context.Context, orderID string, items []ports.InventoryItem) (string, error) {
	f.reservedItems = append([]ports.InventoryItem(nil), items...)
	if f.reservationID == "" {
		f.reservationID = "res-1"
	}
	return f.reservationID, nil
}

func (f *fakeInventory) CommitReservation(ctx context.Context, reservationID string) error {
	f.committed = reservationID
	return nil
}

func (f *fakeInventory) ReleaseReservation(ctx context.Context, reservationID string) error {
	f.released = reservationID
	return nil
}

type recordedPublisher struct {
	subjects []string
}

func (p *recordedPublisher) Publish(ctx context.Context, subject string, event any) error {
	p.subjects = append(p.subjects, subject)
	return nil
}

func TestCreateOrderReservesInventoryAndPublishesEvent(t *testing.T) {
	ctx := context.Background()
	inventory := &fakeInventory{reservationID: "res-123"}
	publisher := &recordedPublisher{}
	svc := service.New(memory.NewRepository(), inventory, publisher)

	order, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items: []domain.OrderItem{
			{SKU: "shoe-1", Quantity: 2, UnitPriceCents: 1250},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	if order.Status != domain.StatusPending {
		t.Fatalf("order status = %q, want pending", order.Status)
	}
	if order.PaymentDueAt.IsZero() {
		t.Fatal("payment_due_at should be set")
	}
	if publisher.subjects[0] != "order.created" {
		t.Fatalf("published subject = %q, want order.created", publisher.subjects[0])
	}
}

func TestMarkOrderPaidThenShipCommitsInventory(t *testing.T) {
	ctx := context.Background()
	inventory := &fakeInventory{reservationID: "res-123"}
	svc := service.New(memory.NewRepository(), inventory)

	order, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "bag-1", Quantity: 1, UnitPriceCents: 5000}},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	order, err = svc.MarkOrderPaid(ctx, order.ID, "pay-1", order.TotalCents)
	if err != nil {
		t.Fatalf("MarkOrderPaid returned error: %v", err)
	}
	if order.Status != domain.StatusPaid {
		t.Fatalf("order status = %q, want paid", order.Status)
	}
	if inventory.committed != "" {
		t.Fatal("inventory should not commit on paid")
	}

	order, err = svc.ShipOrder(ctx, order.ID, "manager-1")
	if err != nil {
		t.Fatalf("ShipOrder returned error: %v", err)
	}
	if order.Status != domain.StatusInTransit {
		t.Fatalf("order status = %q, want in_transit", order.Status)
	}
	if inventory.committed != "res-123" {
		t.Fatalf("committed reservation = %q, want res-123", inventory.committed)
	}
}

func TestCancelPaidOrderReleasesInventory(t *testing.T) {
	ctx := context.Background()
	inventory := &fakeInventory{reservationID: "res-123"}
	svc := service.New(memory.NewRepository(), inventory)

	order, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "clock-1", Quantity: 1, UnitPriceCents: 7500}},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if _, err := svc.MarkOrderPaid(ctx, order.ID, "pay-1", order.TotalCents); err != nil {
		t.Fatalf("MarkOrderPaid returned error: %v", err)
	}

	_, err = svc.CancelOrder(ctx, order.ID)
	if err != nil {
		t.Fatalf("CancelOrder returned error: %v", err)
	}
	if inventory.released != "res-123" {
		t.Fatalf("released reservation = %q, want res-123", inventory.released)
	}
}

func TestCancelInTransitOrderFails(t *testing.T) {
	ctx := context.Background()
	inventory := &fakeInventory{reservationID: "res-123"}
	svc := service.New(memory.NewRepository(), inventory)

	order, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "clock-1", Quantity: 1, UnitPriceCents: 7500}},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if _, err := svc.MarkOrderPaid(ctx, order.ID, "pay-1", order.TotalCents); err != nil {
		t.Fatalf("MarkOrderPaid: %v", err)
	}
	if _, err := svc.ShipOrder(ctx, order.ID, "manager-1"); err != nil {
		t.Fatalf("ShipOrder: %v", err)
	}

	_, err = svc.CancelOrder(ctx, order.ID)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("CancelOrder error = %v, want ErrInvalidTransition", err)
	}
}
