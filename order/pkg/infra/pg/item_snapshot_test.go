package pg

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
)

func snapshotItem() domain.OrderItem {
	return domain.OrderItem{
		SkuID:        "sku-1",
		SKU:          "BAG-001",
		Quantity:     1,
		UnitPriceWon: 1000,
		ProductID:    "prd-galleria",
		ProductName:  "Prada Galleria",
		ImageURL:     "https://cdn.example/bag.jpg",
	}
}

func assertSnapshot(t *testing.T, item domain.OrderItem, where string) {
	t.Helper()
	if item.ProductID != "prd-galleria" || item.ProductName != "Prada Galleria" ||
		item.ImageURL != "https://cdn.example/bag.jpg" {
		t.Fatalf("%s lost the catalog snapshot: %+v", where, item)
	}
}

// The catalog snapshot is what the storefront renders an order line from — its
// image, its name, and the product page it links back to.
func TestOrderItemSnapshotRoundTrips(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_item_snapshot_test")
	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := t.Context()
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	order, err := domain.NewOrder("ord-snap-1", "cust-1", "res-1",
		[]domain.OrderItem{snapshotItem()}, "", 0, 0, now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	if err := repo.Save(ctx, order); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := repo.Get(ctx, "ord-snap-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertSnapshot(t, loaded.Items[0], "Get")

	listed, err := repo.ListByCustomer(ctx, "cust-1")
	if err != nil {
		t.Fatalf("ListByCustomer: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("listed %d orders, want 1", len(listed))
	}
	assertSnapshot(t, listed[0].Items[0], "ListByCustomer")
}

// CancelIfPending returns the order the cancel API responds with. It used to
// load only the pricing columns, so a shopper who canceled watched the names
// and images drop out of the order it handed back.
func TestCancelIfPendingKeepsItemSnapshot(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_cancel_snapshot_test")
	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := t.Context()
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	order, err := domain.NewOrder("ord-snap-2", "cust-1", "res-1",
		[]domain.OrderItem{snapshotItem()}, "", 0, 0, now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	if err := repo.Save(ctx, order); err != nil {
		t.Fatalf("Save: %v", err)
	}

	canceled, ok, err := repo.CancelIfPending(ctx, "ord-snap-2", now, nil)
	if err != nil {
		t.Fatalf("CancelIfPending: %v", err)
	}
	if !ok || canceled == nil {
		t.Fatal("want canceled order")
	}
	assertSnapshot(t, canceled.Items[0], "CancelIfPending")
}

// Same for the paid-order refund path, which the cancel API also answers from.
func TestCancelIfPaidForRefundKeepsItemSnapshot(t *testing.T) {
	dsn := requireDSN(t)
	pool := freshSchema(t, dsn, "order_refund_snapshot_test")
	repo := &Repository{pool: pool}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := t.Context()
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	order, err := domain.NewOrder("ord-snap-3", "cust-1", "res-1",
		[]domain.OrderItem{snapshotItem()}, "", 0, 0, now)
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	if err := order.MarkPaid("pay-snap-3", order.TotalWon, now); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if err := repo.Save(ctx, order); err != nil {
		t.Fatalf("Save: %v", err)
	}

	canceled, ok, err := repo.CancelIfPaidForRefund(ctx, "ord-snap-3", "pay-snap-3", now, nil)
	if err != nil {
		t.Fatalf("CancelIfPaidForRefund: %v", err)
	}
	if !ok || canceled == nil {
		t.Fatal("want canceled order")
	}
	assertSnapshot(t, canceled.Items[0], "CancelIfPaidForRefund")
}
