package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/infra/memory"
	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/elug3/dupli1/order/pkg/service"
)

type fakeStock struct {
	reservedItems []ports.StockItem
	reservationID string
	committed     string
	released      string
}

func (f *fakeStock) Reserve(ctx context.Context, orderID string, items []ports.StockItem) (string, error) {
	f.reservedItems = append([]ports.StockItem(nil), items...)
	if f.reservationID == "" {
		f.reservationID = "res-1"
	}
	return f.reservationID, nil
}

func (f *fakeStock) CommitReservation(ctx context.Context, reservationID string) error {
	f.committed = reservationID
	return nil
}

func (f *fakeStock) ReleaseReservation(ctx context.Context, reservationID string) error {
	f.released = reservationID
	return nil
}

type recordedPublisher struct {
	subjects []string
	events   []any
}

func (p *recordedPublisher) Publish(ctx context.Context, subject string, event any) error {
	p.subjects = append(p.subjects, subject)
	p.events = append(p.events, event)
	return nil
}

func TestCreateOrderReservesStockAndPublishesEvent(t *testing.T) {
	ctx := context.Background()
	stock := &fakeStock{reservationID: "res-123"}
	publisher := &recordedPublisher{}
	svc := service.New(memory.NewRepository() , stock, publisher)

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

func TestMarkOrderPaidThenShipCommitsStock(t *testing.T) {
	ctx := context.Background()
	stock := &fakeStock{reservationID: "res-123"}
	svc := service.New(memory.NewRepository(), stock)

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
	if stock.committed != "" {
		t.Fatal("stock should not commit on paid")
	}

	order, err = svc.ShipOrder(ctx, order.ID, "manager-1")
	if err != nil {
		t.Fatalf("ShipOrder returned error: %v", err)
	}
	if order.Status != domain.StatusInTransit {
		t.Fatalf("order status = %q, want in_transit", order.Status)
	}
	if stock.committed != "res-123" {
		t.Fatalf("committed reservation = %q, want res-123", stock.committed)
	}
}

func TestCancelPaidOrderReleasesStock(t *testing.T) {
	ctx := context.Background()
	stock := &fakeStock{reservationID: "res-123"}
	svc := service.New(memory.NewRepository(), stock)

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
	if stock.released != "res-123" {
		t.Fatalf("released reservation = %q, want res-123", stock.released)
	}
}

func TestCancelInTransitOrderFails(t *testing.T) {
	ctx := context.Background()
	stock := &fakeStock{reservationID: "res-123"}
	svc := service.New(memory.NewRepository(), stock)

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

func TestCreateOrderReservesStockWithSkuID(t *testing.T) {
	ctx := context.Background()
	stock := &fakeStock{reservationID: "res-999"}
	svc := service.New(memory.NewRepository(), stock)

	_, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items: []domain.OrderItem{
			{SkuID: "SKUID-1", SKU: "shoe-1", Quantity: 2, UnitPriceCents: 1250},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if len(stock.reservedItems) != 1 || stock.reservedItems[0].SkuID != "SKUID-1" {
		t.Fatalf("reserved items = %+v, want SkuID SKUID-1 forwarded", stock.reservedItems)
	}
}

func TestCreateOrderEventCarriesSkuID(t *testing.T) {
	ctx := context.Background()
	stock := &fakeStock{reservationID: "res-1"}
	publisher := &recordedPublisher{}
	svc := service.New(memory.NewRepository() , stock, publisher)

	_, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items: []domain.OrderItem{
			{SkuID: "SKUID-2", SKU: "bag-2", Quantity: 1, UnitPriceCents: 5000},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if len(publisher.events) == 0 {
		t.Fatal("expected at least one published event")
	}

	raw, err := json.Marshal(publisher.events[0])
	if err != nil {
		t.Fatalf("marshal published event: %v", err)
	}
	var decoded struct {
		Items []struct {
			SkuID string `json:"sku_id"`
			SKU   string `json:"sku"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal published event: %v", err)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].SkuID != "SKUID-2" || decoded.Items[0].SKU != "BAG-2" {
		t.Fatalf("published event items = %+v, want sku_id SKUID-2 / sku BAG-2", decoded.Items)
	}
}
