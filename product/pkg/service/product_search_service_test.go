package service_test

import (
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/service"
)

func TestSearchProductsNoColorDuplicates(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BOT-001", Name: "Cassette", Category: "bags", Status: "active"},
	}
	store.Variants = []domain.Variant{
		{SKU: "BOT-001-GRN", ProductID: "BOT-001", Color: "Green", Price: 2500, Status: "active"},
		{SKU: "BOT-001-BLK", ProductID: "BOT-001", Color: "Black", Price: 2500, Status: "active"},
	}
	svc := service.NewProductSearchService(store, nil)

	results, err := svc.SearchProducts(map[string]string{"category": "bags"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 parent style, got %d", len(results))
	}
	if len(results[0].AvailableColors) != 2 {
		t.Fatalf("want 2 colors, got %v", results[0].AvailableColors)
	}
	if results[0].Variants != nil {
		t.Fatal("search results should not embed full variants")
	}
}

func TestCreateVariantUnderParent(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BOT-001", Name: "Cassette", Status: "active"},
	}
	store.Variants = []domain.Variant{
		{SKU: "BOT-001", ProductID: "BOT-001", Color: "Green", Price: 2500, Status: "active"},
	}
	svc := service.NewProductSearchService(store, nil)

	v, err := svc.CreateVariant("BOT-001", domain.Variant{Color: "Black", Price: 2500})
	if err != nil {
		t.Fatal(err)
	}
	if v.SKU == "" || v.ProductID != "BOT-001" {
		t.Fatalf("unexpected variant: %+v", v)
	}

	p, err := svc.GetPublicProduct("BOT-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Variants) != 2 {
		t.Fatalf("want 2 variants on PDP, got %d", len(p.Variants))
	}
}
