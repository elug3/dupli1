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
	subject string
	event   any
}

func (p *recordedPublisher) Publish(ctx context.Context, subject string, event any) error {
	p.subject = subject
	p.event = event
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
	if order.TotalCents != 2500 {
		t.Fatalf("order total = %d, want 2500", order.TotalCents)
	}
	if order.ReservationID != "res-123" {
		t.Fatalf("reservation id = %q, want res-123", order.ReservationID)
	}
	if len(inventory.reservedItems) != 1 || inventory.reservedItems[0].SKU != "shoe-1" || inventory.reservedItems[0].Quantity != 2 {
		t.Fatalf("reserved items = %+v, want one shoe-1 quantity 2", inventory.reservedItems)
	}
	if publisher.subject != "order.created" {
		t.Fatalf("published subject = %q, want order.created", publisher.subject)
	}
}

func TestConfirmOrderCommitsInventoryReservation(t *testing.T) {
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

	order, err = svc.ConfirmOrder(ctx, order.ID)
	if err != nil {
		t.Fatalf("ConfirmOrder returned error: %v", err)
	}
	if order.Status != domain.StatusConfirmed {
		t.Fatalf("order status = %q, want confirmed", order.Status)
	}
	if inventory.committed != "res-123" {
		t.Fatalf("committed reservation = %q, want res-123", inventory.committed)
	}
}

func TestCancelConfirmedOrderDoesNotReleaseInventory(t *testing.T) {
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
	if _, err := svc.ConfirmOrder(ctx, order.ID); err != nil {
		t.Fatalf("ConfirmOrder returned error: %v", err)
	}

	_, err = svc.CancelOrder(ctx, order.ID)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("CancelOrder error = %v, want ErrInvalidTransition", err)
	}
	if inventory.released != "" {
		t.Fatalf("released reservation = %q, want empty", inventory.released)
	}
}
