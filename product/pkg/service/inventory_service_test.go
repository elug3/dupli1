package service_test

import (
	"context"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/service"
)

func newInventoryTestService(t *testing.T) (*service.InventoryService, *memory.ProductStore) {
	t.Helper()
	products := memory.NewProductStore()
	products.Products = []domain.Product{{ID: "BOT-001", Name: "Cassette", Status: "active"}}
	products.Variants = []domain.Variant{
		{SkuID: "SKUID-GRN", SKU: "BOT-001-GRN", ProductID: "BOT-001", Color: "Green", Price: 2500, Status: "active"},
	}
	return service.NewInventoryService(memory.NewInventoryStore(), products), products
}

func TestInventoryUpsertAndGetItem_ByEitherIdentifier(t *testing.T) {
	svc, _ := newInventoryTestService(t)
	ctx := context.Background()

	if _, err := svc.UpsertItem(ctx, service.SkuRef{SKU: "BOT-001-GRN"}, 10); err != nil {
		t.Fatalf("UpsertItem by sku: %v", err)
	}

	bySkuID, err := svc.GetItem(ctx, service.SkuRef{SkuID: "SKUID-GRN"})
	if err != nil {
		t.Fatalf("GetItem by skuId: %v", err)
	}
	if bySkuID.Quantity != 10 || bySkuID.SkuID != "SKUID-GRN" || bySkuID.SKU != "BOT-001-GRN" {
		t.Fatalf("unexpected item: %+v", bySkuID)
	}

	bySku, err := svc.GetItem(ctx, service.SkuRef{SKU: "bot-001-grn"})
	if err != nil {
		t.Fatalf("GetItem by sku (case-insensitive): %v", err)
	}
	if bySku.SkuID != "SKUID-GRN" {
		t.Fatalf("expected resolution to canonical skuId, got %+v", bySku)
	}
}

func TestInventoryGetItem_UnknownReference(t *testing.T) {
	svc, _ := newInventoryTestService(t)
	ctx := context.Background()

	if _, err := svc.GetItem(ctx, service.SkuRef{SKU: "NOPE"}); err != service.ErrInvalidSKU {
		t.Fatalf("want ErrInvalidSKU, got %v", err)
	}
	if _, err := svc.GetItem(ctx, service.SkuRef{SkuID: "NOPE"}); err != service.ErrInvalidSKU {
		t.Fatalf("want ErrInvalidSKU, got %v", err)
	}
	if _, err := svc.GetItem(ctx, service.SkuRef{}); err != service.ErrInvalidSKU {
		t.Fatalf("want ErrInvalidSKU for blank ref, got %v", err)
	}
}

func TestInventoryAdjustStock_InsufficientStock(t *testing.T) {
	svc, _ := newInventoryTestService(t)
	ctx := context.Background()

	if _, err := svc.UpsertItem(ctx, service.SkuRef{SkuID: "SKUID-GRN"}, 5); err != nil {
		t.Fatalf("UpsertItem: %v", err)
	}
	if _, err := svc.AdjustStock(ctx, service.SkuRef{SkuID: "SKUID-GRN"}, -10); err != service.ErrInsufficientStock {
		t.Fatalf("want ErrInsufficientStock, got %v", err)
	}
}

func TestInventoryReserve_AggregatesMixedIdentifierReferences(t *testing.T) {
	svc, _ := newInventoryTestService(t)
	ctx := context.Background()

	if _, err := svc.UpsertItem(ctx, service.SkuRef{SkuID: "SKUID-GRN"}, 10); err != nil {
		t.Fatalf("UpsertItem: %v", err)
	}

	// Same underlying variant referenced once by sku, once by skuId — should
	// aggregate into a single reservation line for 7 units, not two lines.
	reservation, err := svc.Reserve(ctx, "order-1", []service.ReservationItemRef{
		{Ref: service.SkuRef{SKU: "BOT-001-GRN"}, Quantity: 3},
		{Ref: service.SkuRef{SkuID: "SKUID-GRN"}, Quantity: 4},
	})
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if len(reservation.Items) != 1 {
		t.Fatalf("want 1 aggregated line, got %d: %+v", len(reservation.Items), reservation.Items)
	}
	if reservation.Items[0].Quantity != 7 || reservation.Items[0].SkuID != "SKUID-GRN" {
		t.Fatalf("unexpected aggregated item: %+v", reservation.Items[0])
	}

	item, err := svc.GetItem(ctx, service.SkuRef{SkuID: "SKUID-GRN"})
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if item.Reserved != 7 || item.Available() != 3 {
		t.Fatalf("unexpected stock after reserve: %+v", item)
	}

	committed, err := svc.CommitReservation(ctx, reservation.ID)
	if err != nil {
		t.Fatalf("CommitReservation: %v", err)
	}
	if committed.Status != domain.ReservationCommitted {
		t.Fatalf("want committed status, got %s", committed.Status)
	}

	item, err = svc.GetItem(ctx, service.SkuRef{SkuID: "SKUID-GRN"})
	if err != nil {
		t.Fatalf("GetItem after commit: %v", err)
	}
	if item.Quantity != 3 || item.Reserved != 0 {
		t.Fatalf("unexpected stock after commit: %+v", item)
	}
}

func TestInventoryReleaseReservation(t *testing.T) {
	svc, _ := newInventoryTestService(t)
	ctx := context.Background()

	if _, err := svc.UpsertItem(ctx, service.SkuRef{SkuID: "SKUID-GRN"}, 10); err != nil {
		t.Fatalf("UpsertItem: %v", err)
	}
	reservation, err := svc.Reserve(ctx, "order-2", []service.ReservationItemRef{
		{Ref: service.SkuRef{SkuID: "SKUID-GRN"}, Quantity: 5},
	})
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	released, err := svc.ReleaseReservation(ctx, reservation.ID)
	if err != nil {
		t.Fatalf("ReleaseReservation: %v", err)
	}
	if released.Status != domain.ReservationReleased {
		t.Fatalf("want released status, got %s", released.Status)
	}

	item, err := svc.GetItem(ctx, service.SkuRef{SkuID: "SKUID-GRN"})
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if item.Quantity != 10 || item.Reserved != 0 {
		t.Fatalf("unexpected stock after release: %+v", item)
	}

	if _, err := svc.CommitReservation(ctx, reservation.ID); err != service.ErrReservationClosed {
		t.Fatalf("want ErrReservationClosed committing a released reservation, got %v", err)
	}
}
