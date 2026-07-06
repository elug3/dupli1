package domain_test

import (
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
)

func TestEnrichFromVariantsSummaries(t *testing.T) {
	p := domain.Product{ID: "BOT-001", Name: "Cassette"}
	variants := []domain.Variant{
		{SKU: "BOT-001-GRN", ProductID: "BOT-001", Color: "Green", SellingPrice: 3200, Price: 2600, Status: "active", ImageURLs: []string{"green.jpg"}},
		{SKU: "BOT-001-BLK", ProductID: "BOT-001", Color: "Black", SellingPrice: 3000, Price: 2500, Status: "active", ImageURLs: []string{"black.jpg"}},
		{SKU: "BOT-001-RED", ProductID: "BOT-001", Color: "Red", SellingPrice: 2900, Price: 2400, Status: "draft"},
	}

	p.EnrichFromVariants(variants, true)

	if len(p.Variants) != 3 {
		t.Fatalf("want all variants embedded, got %d", len(p.Variants))
	}
	if len(p.AvailableColors) != 2 {
		t.Fatalf("want active colors only, got %v", p.AvailableColors)
	}
	if p.PriceFrom != 2500 {
		t.Fatalf("want min active price 2500, got %v", p.PriceFrom)
	}
	if p.SellingPriceFrom != 3000 {
		t.Fatalf("want sellingPriceFrom of cheapest variant 3000, got %v", p.SellingPriceFrom)
	}
	if p.DefaultImageURL != "green.jpg" {
		t.Fatalf("want first active image, got %q", p.DefaultImageURL)
	}
}

func TestEnrichFromVariantsListCardOmitsVariants(t *testing.T) {
	p := domain.Product{ID: "BOT-001"}
	p.EnrichFromVariants([]domain.Variant{
		{SKU: "BOT-001-GRN", Color: "Green", Price: 100, Status: "active"},
	}, false)

	if p.Variants != nil {
		t.Fatalf("list cards should omit variants, got %v", p.Variants)
	}
	if len(p.AvailableColors) != 1 || p.AvailableColors[0] != "Green" {
		t.Fatalf("want availableColors=[Green], got %v", p.AvailableColors)
	}
}
