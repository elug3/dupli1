package pg

import (
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
)

func TestProductSizeChartRoundTrip(t *testing.T) {
	dsn := requireProductDSN(t)
	pool := freshProductSchema(t, dsn, "product_size_chart_test")
	ctx := t.Context()

	store := &ProductSearchStore{pool: pool}
	if err := store.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	stock, err := NewInventoryStore(pool)
	if err != nil {
		t.Fatalf("inventory migrate: %v", err)
	}
	if err := store.SeedMockEcoBag(ctx, stock); err != nil {
		t.Fatalf("seed mock eco-bag: %v", err)
	}
	id, err := store.mockEcoBagProductID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.GetProduct(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if p.SizeChart != nil {
		t.Fatalf("a product without a chart reads back %v, want nil", p.SizeChart)
	}

	p.SizeChart = []domain.SizeChartRow{
		{Size: "M", ChestCm: 112, LengthCm: 70.5},
		{Size: "L", ChestCm: 118, LengthCm: 72, SleeveCm: 64},
	}
	if _, err := store.UpdateProduct(ctx, *p); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := store.GetProduct(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.SizeChart) != 2 || got.SizeChart[0] != p.SizeChart[0] || got.SizeChart[1] != p.SizeChart[1] {
		t.Fatalf("size chart = %+v, want %+v", got.SizeChart, p.SizeChart)
	}

	got.SizeChart = nil
	if _, err := store.UpdateProduct(ctx, *got); err != nil {
		t.Fatalf("clear: %v", err)
	}
	cleared, err := store.GetProduct(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.SizeChart != nil {
		t.Fatalf("cleared chart reads back %v", cleared.SizeChart)
	}
}
