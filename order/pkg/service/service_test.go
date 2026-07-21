package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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

type failingPublisher struct {
	calls int
}

func (p *failingPublisher) Publish(ctx context.Context, subject string, event any) error {
	p.calls++
	return errors.New("nats unavailable")
}

type countingStock struct {
	fakeStock
	reserveCalls int
}

func (f *countingStock) Reserve(ctx context.Context, orderID string, items []ports.StockItem) (string, error) {
	f.reserveCalls++
	return f.fakeStock.Reserve(ctx, orderID, items)
}

// fakeProduct resolves catalog prices; client UnitPriceCents is ignored by the service.
type fakeProduct struct {
	defaultCents int64
	byKey        map[string]*ports.VariantInfo
}

func (f *fakeProduct) GetVariant(_ context.Context, sku string) (*ports.VariantInfo, error) {
	return f.lookup(strings.ToUpper(strings.TrimSpace(sku)), true)
}

func (f *fakeProduct) GetVariantBySkuID(_ context.Context, skuID string) (*ports.VariantInfo, error) {
	return f.lookup(strings.TrimSpace(skuID), false)
}

func (f *fakeProduct) lookup(key string, asSKU bool) (*ports.VariantInfo, error) {
	if f.byKey != nil {
		if v, ok := f.byKey[key]; ok {
			cp := *v
			return &cp, nil
		}
		if v, ok := f.byKey[strings.ToUpper(key)]; ok {
			cp := *v
			return &cp, nil
		}
	}
	cents := f.defaultCents
	if cents == 0 {
		cents = 1000
	}
	if asSKU {
		sku := strings.ToUpper(key)
		return &ports.VariantInfo{SkuID: "ID-" + sku, SKU: sku, UnitPriceCents: cents}, nil
	}
	return &ports.VariantInfo{SkuID: key, SKU: strings.ToUpper(key), UnitPriceCents: cents}, nil
}

func newSvc(stock ports.StockClient, product *fakeProduct, publisher ...ports.EventPublisher) *service.Service {
	if product == nil {
		product = &fakeProduct{}
	}
	return service.New(memory.NewRepository(), stock, publisher...).WithProduct(product)
}

func TestCreateOrderReservesStockAndPublishesEvent(t *testing.T) {
	ctx := context.Background()
	stock := &fakeStock{reservationID: "res-123"}
	publisher := &recordedPublisher{}
	svc := newSvc(stock, &fakeProduct{defaultCents: 1250}, publisher)

	order, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items: []domain.OrderItem{
			{SKU: "shoe-1", Quantity: 2, UnitPriceCents: 1}, // client price ignored
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	if order.Status != domain.StatusPending {
		t.Fatalf("order status = %q, want pending", order.Status)
	}
	if order.TotalCents != 2500 {
		t.Fatalf("total = %d, want 2500 from catalog (not client 1)", order.TotalCents)
	}
	if order.PaymentDueAt.IsZero() {
		t.Fatal("payment_due_at should be set")
	}
	if publisher.subjects[0] != "order.created" {
		t.Fatalf("published subject = %q, want order.created", publisher.subjects[0])
	}
}

func TestCreateOrderIgnoresClientUnitPrice(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(&fakeStock{}, &fakeProduct{defaultCents: 2890000})

	order, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "BAG-1", Quantity: 1, UnitPriceCents: 1}},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if order.Items[0].UnitPriceCents != 2890000 || order.TotalCents != 2890000 {
		t.Fatalf("priced = %+v total=%d, want catalog 2890000", order.Items[0], order.TotalCents)
	}
}

func TestMarkOrderPaidThenShipCommitsStock(t *testing.T) {
	ctx := context.Background()
	stock := &fakeStock{reservationID: "res-123"}
	svc := newSvc(stock, &fakeProduct{defaultCents: 5000})

	order, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
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
	svc := newSvc(stock, &fakeProduct{defaultCents: 7500})

	order, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "clock-1", Quantity: 1}},
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
	svc := newSvc(stock, &fakeProduct{defaultCents: 7500})

	order, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "clock-1", Quantity: 1}},
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
	svc := newSvc(stock, &fakeProduct{
		byKey: map[string]*ports.VariantInfo{
			"SKUID-1": {SkuID: "SKUID-1", SKU: "SHOE-1", UnitPriceCents: 1250},
		},
	})

	_, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items: []domain.OrderItem{
			{SkuID: "SKUID-1", SKU: "shoe-1", Quantity: 2, UnitPriceCents: 1},
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
	svc := newSvc(stock, &fakeProduct{
		byKey: map[string]*ports.VariantInfo{
			"SKUID-2": {SkuID: "SKUID-2", SKU: "BAG-2", UnitPriceCents: 5000},
		},
	}, publisher)

	_, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items: []domain.OrderItem{
			{SkuID: "SKUID-2", SKU: "bag-2", Quantity: 1, UnitPriceCents: 1},
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

func TestCreateOrderIdempotencyKeyReplaysWithoutSecondReserve(t *testing.T) {
	ctx := context.Background()
	stock := &countingStock{fakeStock: fakeStock{reservationID: "res-1"}}
	publisher := &recordedPublisher{}
	svc := newSvc(stock, &fakeProduct{defaultCents: 1000}, publisher)

	input := service.CreateOrderInput{
		CustomerID:     "customer-1",
		IdempotencyKey: "idem-abc",
		Items:          []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
	}
	first, err := svc.CreateOrder(ctx, input)
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	second, err := svc.CreateOrder(ctx, input)
	if err != nil {
		t.Fatalf("CreateOrder replay: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("replay order id = %q, want %q", second.ID, first.ID)
	}
	if stock.reserveCalls != 1 {
		t.Fatalf("reserve calls = %d, want 1", stock.reserveCalls)
	}
}

func TestCreateOrderIdempotencyKeyConflict(t *testing.T) {
	ctx := context.Background()
	svc := newSvc(&fakeStock{reservationID: "res-1"}, &fakeProduct{defaultCents: 1000})

	_, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID:     "customer-1",
		IdempotencyKey: "idem-conflict",
		Items:          []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	_, err = svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID:     "customer-1",
		IdempotencyKey: "idem-conflict",
		Items:          []domain.OrderItem{{SKU: "bag-2", Quantity: 1}},
	})
	if !errors.Is(err, ports.ErrIdempotencyConflict) {
		t.Fatalf("error = %v, want ErrIdempotencyConflict", err)
	}
}

func TestCreateOrderSucceedsWhenPublishFails(t *testing.T) {
	ctx := context.Background()
	stock := &fakeStock{reservationID: "res-1"}
	publisher := &failingPublisher{}
	repo := memory.NewRepository()
	svc := service.New(repo, stock, publisher).WithProduct(&fakeProduct{defaultCents: 1000})

	order, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("CreateOrder should soft-succeed: %v", err)
	}
	if order.ID == "" {
		t.Fatal("expected order id")
	}
	if publisher.calls < 1 {
		t.Fatal("expected publish attempt")
	}
	pending, err := repo.ListPendingOutbox(ctx, 10)
	if err != nil {
		t.Fatalf("ListPendingOutbox: %v", err)
	}
	if len(pending) != 1 || pending[0].Subject != "order.created" {
		t.Fatalf("pending outbox = %+v, want one order.created", pending)
	}
}

func TestDrainOutboxPublishesPending(t *testing.T) {
	ctx := context.Background()
	stock := &fakeStock{reservationID: "res-1"}
	failPub := &failingPublisher{}
	repo := memory.NewRepository()
	svc := service.New(repo, stock, failPub).WithProduct(&fakeProduct{defaultCents: 1000})

	if _, err := svc.CreateOrder(ctx, service.CreateOrderInput{
		CustomerID: "customer-1",
		Items:      []domain.OrderItem{{SKU: "bag-1", Quantity: 1}},
	}); err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	okPub := &recordedPublisher{}
	svcOK := service.New(repo, stock, okPub).WithProduct(&fakeProduct{defaultCents: 1000})
	if err := svcOK.DrainOutbox(ctx); err != nil {
		t.Fatalf("DrainOutbox: %v", err)
	}
	if len(okPub.subjects) != 1 || okPub.subjects[0] != "order.created" {
		t.Fatalf("subjects = %v, want [order.created]", okPub.subjects)
	}
	pending, err := repo.ListPendingOutbox(ctx, 10)
	if err != nil {
		t.Fatalf("ListPendingOutbox: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %d, want 0", len(pending))
	}
}
