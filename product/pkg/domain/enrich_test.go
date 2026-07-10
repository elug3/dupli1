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

func TestVariantMergeUpdate_PartialBodyKeepsOmittedFields(t *testing.T) {
	existing := domain.Variant{
		SkuID:        "SKUID-1",
		SKU:          "BOT-001-GRN",
		ProductID:    "BOT-001",
		Color:        "Green",
		Size:         "M",
		SellingPrice: 3200,
		Price:        2600,
		Status:       "draft",
		ImageURLs:    []string{"green.jpg"},
		CreatedAt:    "2026-01-01T00:00:00Z",
	}

	// Price-only update — everything else must survive untouched.
	merged := existing.MergeUpdate(domain.Variant{Price: 2500})

	if merged.Price != 2500 {
		t.Fatalf("price = %v, want 2500", merged.Price)
	}
	if merged.Color != "Green" || merged.Size != "M" {
		t.Fatalf("color/size were clobbered: %+v", merged)
	}
	if merged.Status != "draft" {
		t.Fatalf("status = %q, want draft (must not reactivate on unrelated update)", merged.Status)
	}
	if merged.SellingPrice != 3200 {
		t.Fatalf("sellingPrice = %v, want 3200", merged.SellingPrice)
	}
	if len(merged.ImageURLs) != 1 || merged.ImageURLs[0] != "green.jpg" {
		t.Fatalf("imageURLs = %v, want [green.jpg]", merged.ImageURLs)
	}
	// Identity fields always come from existing.
	if merged.SkuID != "SKUID-1" || merged.SKU != "BOT-001-GRN" || merged.ProductID != "BOT-001" {
		t.Fatalf("identity fields changed: %+v", merged)
	}
}

func TestVariantMergeUpdate_FullBodyReplacesEverything(t *testing.T) {
	existing := domain.Variant{
		SKU: "BOT-001-GRN", Color: "Green", Size: "M", Price: 2600, Status: "draft",
		ImageURLs: []string{"green.jpg"},
	}

	merged := existing.MergeUpdate(domain.Variant{
		Color: "Black", Size: "L", Price: 2700, Status: "active",
		ImageURLs: []string{"black.jpg"},
	})

	if merged.Color != "Black" || merged.Size != "L" || merged.Price != 2700 || merged.Status != "active" {
		t.Fatalf("full update did not apply: %+v", merged)
	}
	if len(merged.ImageURLs) != 1 || merged.ImageURLs[0] != "black.jpg" {
		t.Fatalf("imageURLs = %v, want [black.jpg]", merged.ImageURLs)
	}
}
