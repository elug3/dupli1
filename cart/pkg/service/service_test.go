package service_test

import (
	"context"
	"testing"

	"github.com/elug3/dupli1/cart/pkg/infra/memory"
	"github.com/elug3/dupli1/cart/pkg/ports"
	"github.com/elug3/dupli1/cart/pkg/service"
)

type fakeProductClient struct {
	bySKU   map[string]*ports.VariantInfo
	bySkuID map[string]*ports.VariantInfo
}

func (f *fakeProductClient) GetVariant(_ context.Context, sku string) (*ports.VariantInfo, error) {
	info, ok := f.bySKU[sku]
	if !ok {
		return nil, ports.ErrVariantNotFound
	}
	copied := *info
	return &copied, nil
}

func (f *fakeProductClient) GetVariantBySkuID(_ context.Context, skuID string) (*ports.VariantInfo, error) {
	info, ok := f.bySkuID[skuID]
	if !ok {
		return nil, ports.ErrVariantNotFound
	}
	copied := *info
	return &copied, nil
}

type fakeInventoryClient struct {
	bySKU   map[string]int
	bySkuID map[string]int
}

func (f *fakeInventoryClient) GetAvailableQty(_ context.Context, sku string) (int, error) {
	return f.bySKU[sku], nil
}

func (f *fakeInventoryClient) GetAvailableQtyBySkuID(_ context.Context, skuID string) (int, error) {
	return f.bySkuID[skuID], nil
}

func newTestService(t *testing.T) *service.Service {
	t.Helper()
	variant := &ports.VariantInfo{
		SkuID:          "SKUID-GRN",
		SKU:            "BOT-001-GRN",
		ProductID:      "BOT-001",
		Color:          "Green",
		UnitPriceCents: 250000,
		ImageURL:       "https://example.com/green.jpg",
	}
	product := &fakeProductClient{
		bySKU:   map[string]*ports.VariantInfo{"BOT-001-GRN": variant},
		bySkuID: map[string]*ports.VariantInfo{"SKUID-GRN": variant},
	}
	inventory := &fakeInventoryClient{
		bySKU:   map[string]int{"BOT-001-GRN": 7},
		bySkuID: map[string]int{"SKUID-GRN": 7},
	}
	return service.New(memory.NewRepository(), product, inventory)
}

func TestUpsertItem_BySKU_PersistsResolvedSkuID(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	cart, err := svc.UpsertItem(ctx, "cust-1", service.ItemInput{SKU: "bot-001-grn", Quantity: 2})
	if err != nil {
		t.Fatalf("UpsertItem: %v", err)
	}
	if len(cart.Items) != 1 {
		t.Fatalf("want 1 item, got %d", len(cart.Items))
	}
	item := cart.Items[0]
	if item.SkuID != "SKUID-GRN" || item.SKU != "BOT-001-GRN" {
		t.Fatalf("want resolved skuId+sku, got %+v", item)
	}
	if item.AvailableQty != 7 || item.UnitPriceCents != 250000 {
		t.Fatalf("unexpected enrichment: %+v", item)
	}
}

func TestUpsertItem_BySkuID_PersistsResolvedSKU(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	cart, err := svc.UpsertItem(ctx, "cust-2", service.ItemInput{SkuID: "SKUID-GRN", Quantity: 1})
	if err != nil {
		t.Fatalf("UpsertItem: %v", err)
	}
	item := cart.Items[0]
	if item.SkuID != "SKUID-GRN" || item.SKU != "BOT-001-GRN" {
		t.Fatalf("want resolved skuId+sku, got %+v", item)
	}
}

func TestUpsertItem_UnknownSkuID_ReturnsNotFound(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	if _, err := svc.UpsertItem(ctx, "cust-3", service.ItemInput{SkuID: "NOPE", Quantity: 1}); err != ports.ErrVariantNotFound {
		t.Fatalf("want ErrVariantNotFound, got %v", err)
	}
}

func TestReplaceItems_MixedIdentifiers(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	cart, err := svc.ReplaceItems(ctx, "cust-4", []service.ItemInput{
		{SkuID: "SKUID-GRN", Quantity: 3},
	})
	if err != nil {
		t.Fatalf("ReplaceItems: %v", err)
	}
	if len(cart.Items) != 1 || cart.Items[0].SKU != "BOT-001-GRN" {
		t.Fatalf("unexpected cart: %+v", cart.Items)
	}
}

func TestRemoveItemBySkuID(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	if _, err := svc.UpsertItem(ctx, "cust-5", service.ItemInput{SkuID: "SKUID-GRN", Quantity: 1}); err != nil {
		t.Fatalf("UpsertItem: %v", err)
	}
	cart, err := svc.RemoveItemBySkuID(ctx, "cust-5", "SKUID-GRN")
	if err != nil {
		t.Fatalf("RemoveItemBySkuID: %v", err)
	}
	if len(cart.Items) != 0 {
		t.Fatalf("want empty cart, got %+v", cart.Items)
	}
}

func TestGetCart_EnrichesStoredItemBySkuIDWhenPresent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	if _, err := svc.UpsertItem(ctx, "cust-6", service.ItemInput{SKU: "BOT-001-GRN", Quantity: 4}); err != nil {
		t.Fatalf("UpsertItem: %v", err)
	}

	cart, err := svc.GetCart(ctx, "cust-6")
	if err != nil {
		t.Fatalf("GetCart: %v", err)
	}
	if len(cart.Items) != 1 || cart.Items[0].SkuID != "SKUID-GRN" || cart.Items[0].AvailableQty != 7 {
		t.Fatalf("unexpected cart: %+v", cart.Items)
	}
}
