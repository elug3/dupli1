package service_test

import (
	"fmt"
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

	results, _, err := svc.SearchProducts(map[string]string{"category": "bags"}, true)
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
	if _, err := store.Catalog.CreateStyle(domain.Style{BrandCode: "BOT", Code: "CAS001", Name: "Cassette"}); err != nil {
		t.Fatal(err)
	}
	store.Products = []domain.Product{
		{ID: "BOT-001", Name: "Cassette", BrandCode: "BOT", StyleCode: "CAS001", Status: "active"},
	}
	store.Variants = []domain.Variant{
		{SKU: "BOT_CAS001_GRN_OS", ProductID: "BOT-001", Color: "Green", ColorCode: "GRN", SizeCode: "OS", Price: 2500, Status: "active"},
	}
	svc := service.NewProductSearchService(store, nil)

	v, err := svc.CreateVariant("BOT-001", domain.Variant{Color: "Black", Price: 2500})
	if err != nil {
		t.Fatal(err)
	}
	if v.SKU != "BOT_CAS001_BLK_OS" || v.ProductID != "BOT-001" {
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

func TestUpdateVariant_PartialBodyDoesNotClearOtherFields(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BOT-001", Name: "Cassette", Status: "active"},
	}
	store.Variants = []domain.Variant{
		{
			SkuID: "SKUID-1", SKU: "BOT-001-GRN", ProductID: "BOT-001",
			Color: "Green", Size: "M", Price: 2500, Status: "draft",
			ImageURLs: []string{"green.jpg"},
		},
	}
	svc := service.NewProductSearchService(store, nil)

	// Price-only update, as an admin PUT would send if the client only
	// changed the price field on the form.
	updated, err := svc.UpdateVariant("BOT-001", "BOT-001-GRN", domain.Variant{Price: 2600})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Price != 2600 {
		t.Fatalf("price = %v, want 2600", updated.Price)
	}
	if updated.Color != "Green" || updated.Size != "M" {
		t.Fatalf("color/size were wiped by a partial update: %+v", updated)
	}
	if updated.Status != "draft" {
		t.Fatalf("status = %q, want draft to survive an unrelated field update", updated.Status)
	}
	if len(updated.ImageURLs) != 1 || updated.ImageURLs[0] != "green.jpg" {
		t.Fatalf("imageURLs = %v, want [green.jpg] preserved", updated.ImageURLs)
	}
	if updated.SkuID != "SKUID-1" {
		t.Fatalf("skuId changed on update: %+v", updated)
	}

	// A draft variant that lost its status would silently vanish from public
	// PDP filtering — confirm it's still there under its still-draft status
	// (i.e. GetPublicVariant correctly still rejects it as non-active).
	if _, err := svc.GetPublicVariant("BOT-001-GRN"); err == nil {
		t.Fatal("draft variant should not be publicly visible")
	}
}

func TestGetPublicVariantsBySkuIDs(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BOT-001", Name: "Cassette", Status: "active"},
		{ID: "BOT-002", Name: "Draft Bag", Status: "draft"},
	}
	store.Variants = []domain.Variant{
		{SkuID: "ID-A", SKU: "BOT-001-BLK", ProductID: "BOT-001", Color: "Black", Price: 2500, Status: "active"},
		{SkuID: "ID-B", SKU: "BOT-001-GRN", ProductID: "BOT-001", Color: "Green", Price: 2500, Status: "draft"},
		{SkuID: "ID-C", SKU: "BOT-002-BLK", ProductID: "BOT-002", Color: "Black", Price: 3000, Status: "active"},
	}
	svc := service.NewProductSearchService(store, nil)

	items, missing, err := svc.GetPublicVariantsBySkuIDs([]string{"ID-A", "ID-B", "ID-C", "ID-MISSING", "ID-A", "  "})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].SkuID != "ID-A" {
		t.Fatalf("items = %+v, want only ID-A", items)
	}
	wantMissing := []string{"ID-B", "ID-C", "ID-MISSING"}
	if len(missing) != len(wantMissing) {
		t.Fatalf("missing = %v, want %v", missing, wantMissing)
	}
	for i, id := range wantMissing {
		if missing[i] != id {
			t.Fatalf("missing[%d] = %q, want %q", i, missing[i], id)
		}
	}

	if _, _, err := svc.GetPublicVariantsBySkuIDs(nil); err == nil {
		t.Fatal("empty sku_ids should be invalid")
	}

	tooMany := make([]string, service.MaxBatchSkuIDs+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("ID-%d", i)
	}
	if _, _, err := svc.GetPublicVariantsBySkuIDs(tooMany); err == nil {
		t.Fatal("oversized batch should be invalid")
	}
}
