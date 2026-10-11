package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/service"
)

func catalogReportFixture(t *testing.T) *service.CatalogReportService {
	t.Helper()
	ctx := context.Background()
	inv := memory.NewInventoryStore()
	store := memory.NewProductStore().WithInventory(inv)
	store.Catalog.Brands = map[string]domain.Brand{} // only the brands below
	for _, b := range []domain.Brand{{Code: "PRD", Name: "Prada"}, {Code: "GUC", Name: "Gucci"}, {Code: "FND", Name: "Fendi"}} {
		if _, err := store.Catalog.CreateBrand(ctx, b); err != nil {
			t.Fatalf("CreateBrand: %v", err)
		}
	}
	store.Products = []domain.Product{
		{ID: "p1", BrandCode: "PRD", Category: "bags", SubCategory: "tote", Status: "active"},
		{ID: "p2", BrandCode: "PRD", Category: "bags", SubCategory: "tote", Status: "draft"},
		{ID: "p3", BrandCode: "PRD", Category: "clothing", SubCategory: "padded", Status: "active"},
		{ID: "p4", BrandCode: "GUC", Category: "bags", SubCategory: "mini", Status: "active"},
		// A legacy row: a brand named without a master, and no subcategory.
		{ID: "p5", Brand: "Vintage", Category: "bags", Status: "active"},
	}
	store.Variants = []domain.Variant{
		{SkuID: "s1", ProductID: "p1"}, {SkuID: "s2", ProductID: "p1"},
		{SkuID: "s3", ProductID: "p2"},
		{SkuID: "s4", ProductID: "p3"},
		{SkuID: "s5", ProductID: "p4"},
	}
	for skuID, qty := range map[string][2]int{"s1": {3, 1}, "s2": {2, 0}, "s3": {5, 0}, "s4": {4, 2}} {
		if err := inv.SaveItem(ctx, &domain.StockItem{SkuID: skuID, Quantity: qty[0], Reserved: qty[1]}); err != nil {
			t.Fatalf("SaveItem: %v", err)
		}
	}
	return service.NewCatalogReportService(store, store.Catalog)
}

func TestCatalogReport_CountsByBrandCategoryAndCell(t *testing.T) {
	report, err := catalogReportFixture(t).Report(context.Background(), "")
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	want := domain.CatalogTally{Products: 5, SKUs: 5, Units: 14, Reserved: 3}
	if report.Totals != want {
		t.Fatalf("totals=%+v, want %+v", report.Totals, want)
	}
	if report.ProductsByStatus["active"] != 4 || report.ProductsByStatus["draft"] != 1 || report.ProductsByStatus["archived"] != 0 {
		t.Fatalf("by status=%v", report.ProductsByStatus)
	}

	brands := map[string]domain.CatalogBrandCount{}
	for _, b := range report.Brands {
		brands[b.Name] = b
	}
	if report.Brands[0].Name != "Prada" || brands["Prada"].Products != 3 || brands["Prada"].SKUs != 4 || brands["Prada"].Units != 14 {
		t.Fatalf("prada=%+v (first=%s)", brands["Prada"], report.Brands[0].Name)
	}
	if f, ok := brands["Fendi"]; !ok || f.Products != 0 || f.Code != "FND" {
		t.Fatalf("an empty brand master must be listed with zeros, got %+v (ok=%v)", f, ok)
	}
	if v, ok := brands["Vintage"]; !ok || v.Code != "" || v.Products != 1 {
		t.Fatalf("a brand without a master must be listed by name, got %+v (ok=%v)", v, ok)
	}

	if len(report.Categories) != 2 || report.Categories[0].Code != "bags" || report.Categories[1].Code != "clothing" {
		t.Fatalf("categories=%+v", report.Categories)
	}
	bags := report.Categories[0]
	if bags.Products != 4 {
		t.Fatalf("bags products=%d, want 4", bags.Products)
	}
	subs := map[string]domain.CatalogSubCategoryCount{}
	for _, s := range bags.SubCategories {
		subs[s.Code] = s
	}
	if subs["tote"].Products != 2 || subs["mini"].Products != 1 || subs[""].Products != 1 {
		t.Fatalf("bag subcategories=%+v", bags.SubCategories)
	}
	if s, ok := subs["handbags"]; !ok || s.Products != 0 || s.Name != "Handbags" {
		t.Fatalf("an empty taxonomy subcategory must be listed with zeros, got %+v", s)
	}

	var prdTote *domain.CatalogCell
	for i, c := range report.Cells {
		if c.Products == 0 {
			t.Fatalf("empty cell listed: %+v", c)
		}
		if c.BrandCode == "PRD" && c.SubCategory == "tote" {
			prdTote = &report.Cells[i]
		}
	}
	if prdTote == nil || prdTote.Products != 2 || prdTote.Category != "bags" || prdTote.Brand != "Prada" {
		t.Fatalf("Prada × tote = %+v", prdTote)
	}
}

func TestCatalogReport_StatusNarrowsCountsButNotStatusTotals(t *testing.T) {
	svc := catalogReportFixture(t)
	report, err := svc.Report(context.Background(), "draft")
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if report.Status != "draft" || report.Totals.Products != 1 || report.Totals.Units != 5 {
		t.Fatalf("draft report totals=%+v status=%q", report.Totals, report.Status)
	}
	if report.ProductsByStatus["active"] != 4 {
		t.Fatalf("products_by_status must cover the whole catalog, got %v", report.ProductsByStatus)
	}
	if _, err := svc.Report(context.Background(), "deleted"); !errors.Is(err, service.ErrInvalidProductStatus) {
		t.Fatalf("unknown status err=%v", err)
	}
}
