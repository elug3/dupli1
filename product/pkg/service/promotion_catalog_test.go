package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/elug3/dupli1/product/pkg/service"
)

// fakeCatalog records what it was asked and answers from a fixed table.
type fakeCatalog struct {
	bySkuID map[string]ports.LineCatalog
	calls   int
	err     error
}

func (c *fakeCatalog) LineAttributes(
	_ context.Context,
	refs []ports.LineRef,
) ([]ports.LineCatalog, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	out := make([]ports.LineCatalog, len(refs))
	for i, ref := range refs {
		if entry, ok := c.bySkuID[ref.SkuID]; ok {
			out[i] = entry
		}
	}
	return out, nil
}

func brandCatalog() *fakeCatalog {
	return &fakeCatalog{bySkuID: map[string]ports.LineCatalog{
		"sku-prada": {
			Found:     true,
			SkuID:     "sku-prada",
			ProductID: "parent-prada",
			Category:  "bags",
			BrandCode: "PRADA",
		},
		"sku-gucci-sale": {
			Found:     true,
			SkuID:     "sku-gucci-sale",
			ProductID: "parent-gucci",
			Category:  "wallets",
			BrandCode: "GUCCI",
			OnSale:    true,
		},
	}}
}

func svcWithCatalog(t *testing.T, catalog ports.PromotionCatalog) (*service.PromotionService, *memory.PromotionStore) {
	t.Helper()
	store := memory.NewPromotionStore()
	svc := service.NewPromotionService(store).
		WithLedger(memory.NewPromotionRedemptionStore(store)).
		WithEntitlements(memory.NewPromotionEntitlementStore()).
		WithCatalog(catalog)
	return svc, store
}

// brandScoped is a code that only applies to lines of a given brand — the
// shape a manager can author but nothing could evaluate before, because a
// checkout carries no brand.
func brandScoped(code, brandCode string) domain.Promotion {
	p := fixedPromotion(code, 10000)
	p.Conditions = domain.Conditions{
		Version:   domain.ConditionsVersion,
		All:       []domain.Predicate{{Attr: domain.AttrLineBrandCode, Op: domain.OpIn, Value: []any{brandCode}}},
		LineMatch: domain.LineMatchAny,
	}
	return p
}

// A line as checkout actually sends it: identity, quantity, price, nothing else.
func line(skuID string, priceWon int64) domain.EvaluationLine {
	return domain.EvaluationLine{SkuID: skuID, SKU: skuID, Quantity: 1, UnitPriceWon: priceWon}
}

func cartOf(lines ...domain.EvaluationLine) domain.EvaluationContext {
	return domain.EvaluationContext{CustomerID: "cust-1", ShippingFeeWon: 30000, Lines: lines}
}

func TestBrandConditionMatchesFromTheCatalog(t *testing.T) {
	catalog := brandCatalog()
	svc, _ := svcWithCatalog(t, catalog)
	if _, err := svc.Create(context.Background(), brandScoped("PRADA10", "PRADA")); err != nil {
		t.Fatalf("create: %v", err)
	}

	result := svc.Evaluate(context.Background(), "PRADA10", cartOf(line("sku-prada", 1000000)))
	if !result.OK {
		t.Fatalf("a Prada line should earn a Prada code, got reason %q/%q", result.Reason, result.SubReason)
	}
	if result.DiscountWon != 10000 {
		t.Fatalf("discount = %d, want 10000", result.DiscountWon)
	}
	if catalog.calls != 1 {
		t.Fatalf("catalog called %d times, want 1", catalog.calls)
	}
}

func TestBrandConditionRefusesAnotherBrand(t *testing.T) {
	svc, _ := svcWithCatalog(t, brandCatalog())
	if _, err := svc.Create(context.Background(), brandScoped("PRADA10", "PRADA")); err != nil {
		t.Fatalf("create: %v", err)
	}

	result := svc.Evaluate(context.Background(), "PRADA10", cartOf(line("sku-gucci-sale", 1000000)))
	if result.OK {
		t.Fatal("a Gucci line should not earn a Prada code")
	}
	if result.Reason != domain.ReasonNotEligible || result.SubReason != domain.SubReasonBrand {
		t.Fatalf("reason = %q/%q, want not_eligible/brand", result.Reason, result.SubReason)
	}
}

// The catalog is the seller's own data, so what the caller claims about a line
// cannot buy a discount.
func TestCallerSuppliedBrandIsOverwritten(t *testing.T) {
	svc, _ := svcWithCatalog(t, brandCatalog())
	if _, err := svc.Create(context.Background(), brandScoped("PRADA10", "PRADA")); err != nil {
		t.Fatalf("create: %v", err)
	}

	claimed := line("sku-gucci-sale", 1000000)
	claimed.BrandCode = "PRADA"
	claimed.Category = "bags"
	if result := svc.Evaluate(context.Background(), "PRADA10", cartOf(claimed)); result.OK {
		t.Fatal("a claimed brand should be replaced by the catalog's, not trusted")
	}
}

func TestCategoryConditionMatchesFromTheCatalog(t *testing.T) {
	svc, _ := svcWithCatalog(t, brandCatalog())
	p := fixedPromotion("BAGS10", 10000)
	p.Conditions = domain.Conditions{
		Version: domain.ConditionsVersion,
		All: []domain.Predicate{
			{Attr: domain.AttrLineCategory, Op: domain.OpIn, Value: []any{"bags"}},
		},
	}
	if _, err := svc.Create(context.Background(), p); err != nil {
		t.Fatalf("create: %v", err)
	}

	if result := svc.Evaluate(context.Background(), "BAGS10", cartOf(line("sku-prada", 500000))); !result.OK {
		t.Fatalf("a bags line should earn a bags code, got %q/%q", result.Reason, result.SubReason)
	}
	if result := svc.Evaluate(context.Background(), "BAGS10", cartOf(line("sku-gucci-sale", 500000))); result.OK {
		t.Fatal("a wallets line should not earn a bags code")
	}
}

// The on-sale exclusion the admin offers as a checkbox: a marked-down line is
// dropped from the discount base, and a cart of only such lines earns nothing.
func TestOnSaleExclusionUsesTheCatalogsSaleState(t *testing.T) {
	svc, _ := svcWithCatalog(t, brandCatalog())
	p := fixedPromotion("FULLPRICE", 10000)
	p.Conditions = domain.Conditions{
		Version: domain.ConditionsVersion,
		Exclude: []domain.Predicate{{Attr: domain.AttrLineOnSale, Op: domain.OpEq, Value: true}},
	}
	if _, err := svc.Create(context.Background(), p); err != nil {
		t.Fatalf("create: %v", err)
	}

	if result := svc.Evaluate(context.Background(), "FULLPRICE", cartOf(line("sku-prada", 500000))); !result.OK {
		t.Fatalf("a full-price line should qualify, got %q/%q", result.Reason, result.SubReason)
	}
	result := svc.Evaluate(context.Background(), "FULLPRICE", cartOf(line("sku-gucci-sale", 500000)))
	if result.OK {
		t.Fatal("a cart of only on-sale lines should not earn the code")
	}
	if result.SubReason != domain.SubReasonOnSale {
		t.Fatalf("sub-reason = %q, want %q", result.SubReason, domain.SubReasonOnSale)
	}
}

// Most codes gate on money alone; those must not pay for a catalog read.
func TestMoneyOnlyConditionsSkipTheCatalog(t *testing.T) {
	catalog := brandCatalog()
	svc, _ := svcWithCatalog(t, catalog)
	p := fixedPromotion("SPEND", 10000)
	p.Conditions = domain.Conditions{
		Version: domain.ConditionsVersion,
		All:     []domain.Predicate{{Attr: domain.AttrSubtotalWon, Op: domain.OpGte, Value: 100000}},
	}
	if _, err := svc.Create(context.Background(), p); err != nil {
		t.Fatalf("create: %v", err)
	}

	if result := svc.Evaluate(context.Background(), "SPEND", cartOf(line("sku-prada", 500000))); !result.OK {
		t.Fatalf("expected eligible, got %q/%q", result.Reason, result.SubReason)
	}
	if catalog.calls != 0 {
		t.Fatalf("catalog called %d times for a money-only code, want 0", catalog.calls)
	}
}

// A lookup failure must refuse the code rather than hand out a discount the
// cart may not have earned.
func TestCatalogFailureRefusesACatalogCondition(t *testing.T) {
	catalog := &fakeCatalog{err: errors.New("database down")}
	svc, _ := svcWithCatalog(t, catalog)
	if _, err := svc.Create(context.Background(), brandScoped("PRADA10", "PRADA")); err != nil {
		t.Fatalf("create: %v", err)
	}

	if result := svc.Evaluate(context.Background(), "PRADA10", cartOf(line("sku-prada", 1000000))); result.OK {
		t.Fatal("a code whose conditions could not be evaluated must be refused")
	}
}

// A code carrying catalog conditions on a service with no catalog wired is the
// pre-plumbing behaviour, and must stay a refusal rather than a free discount.
func TestNoCatalogRefusesACatalogCondition(t *testing.T) {
	svc, _ := newPromotionSvc(t)
	if _, err := svc.Create(context.Background(), brandScoped("PRADA10", "PRADA")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if result := svc.Evaluate(context.Background(), "PRADA10", cartOf(line("sku-prada", 1000000))); result.OK {
		t.Fatal("without a catalog a brand condition cannot match")
	}
}

// Reserve re-evaluates, so the discount it records is the enriched one.
func TestReserveEnrichesTheSameWay(t *testing.T) {
	svc, _ := svcWithCatalog(t, brandCatalog())
	if _, err := svc.Create(context.Background(), brandScoped("PRADA10", "PRADA")); err != nil {
		t.Fatalf("create: %v", err)
	}

	row, result, err := svc.Reserve(
		context.Background(),
		"PRADA10",
		"order-1",
		cartOf(line("sku-prada", 1000000)),
	)
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if !result.OK || row == nil {
		t.Fatalf("reserve refused an eligible cart: %q/%q", result.Reason, result.SubReason)
	}
	if row.DiscountWon != 10000 {
		t.Fatalf("reserved discount = %d, want 10000", row.DiscountWon)
	}
}

// seedMasters registers the SKU master codes a product/variant create needs.
func seedMasters(t *testing.T, store *memory.ProductStore, brand, style, color, size string) {
	t.Helper()
	ctx := context.Background()
	// The store ships with some masters already; only the missing ones matter.
	keep := func(what string, err error) {
		if err != nil && !errors.Is(err, domain.ErrMasterExists) {
			t.Fatalf("seed %s: %v", what, err)
		}
	}
	_, err := store.Catalog.CreateBrand(ctx, domain.Brand{Code: brand, Name: brand})
	keep("brand", err)
	_, err = store.Catalog.CreateStyle(ctx, domain.Style{BrandCode: brand, Code: style, Name: style})
	keep("style", err)
	_, err = store.Catalog.CreateColor(ctx, domain.Color{Code: color, Name: color})
	keep("color", err)
	_, err = store.Catalog.CreateSize(ctx, domain.Size{Code: size, Name: size})
	keep("size", err)
}

// ── StorePromotionCatalog over a real product store ─────────────────────────

func TestStoreCatalogReadsBrandCategoryAndSaleStateFromTheParent(t *testing.T) {
	ctx := context.Background()
	store := memory.NewProductStore()
	seedMasters(t, store, "PRA", "GALLERIA", "BLK", "M")
	parent, err := store.CreateProduct(ctx, domain.Product{
		ID:            "parent-1",
		Name:          "Galleria",
		Brand:         "Prada",
		BrandCode:     "PRA",
		StyleCode:     "GALLERIA",
		Category:      "bags",
		Price:         900000,
		OfficialPrice: 1200000, // marked down → on sale
		Status:        "active",
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	variant, err := store.CreateVariant(ctx, domain.Variant{
		SkuID:     "sku-1",
		SKU:       "PRA_GALLERIA_BLK_M",
		ProductID: parent.ID,
		Color:     "black",
		ColorCode: "BLK",
		SizeCode:  "M",
		Status:    "active",
	})
	if err != nil {
		t.Fatalf("create variant: %v", err)
	}

	catalog := service.NewStorePromotionCatalog(store)
	got, err := catalog.LineAttributes(ctx, []ports.LineRef{
		{SkuID: variant.SkuID, SKU: variant.SKU},
		{SkuID: "", SKU: variant.SKU}, // older cart: human SKU only
		{SkuID: "nope", SKU: "nope"},
	})
	if err != nil {
		t.Fatalf("line attributes: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3", len(got))
	}
	for i, entry := range got[:2] {
		if !entry.Found {
			t.Fatalf("entry %d: not found", i)
		}
		if entry.BrandCode != "PRA" || entry.Category != "bags" {
			t.Fatalf("entry %d: brand/category = %q/%q", i, entry.BrandCode, entry.Category)
		}
		if !entry.OnSale {
			t.Fatalf("entry %d: officialPrice above price should read as on sale", i)
		}
		if entry.ProductID != "parent-1" {
			t.Fatalf("entry %d: productId = %q", i, entry.ProductID)
		}
	}
	if got[2].Found {
		t.Fatal("an unknown sku should come back unfound, not error")
	}
}

func TestStoreCatalogReadsFullPriceAsNotOnSale(t *testing.T) {
	ctx := context.Background()
	store := memory.NewProductStore()
	seedMasters(t, store, "GUC", "CLASSIC", "TAN", "M")
	if _, err := store.CreateProduct(ctx, domain.Product{
		ID: "parent-2", Name: "Classic", Category: "bags", BrandCode: "GUC", StyleCode: "CLASSIC",
		Price: 1000000, OfficialPrice: 1000000, Status: "active",
	}); err != nil {
		t.Fatalf("create product: %v", err)
	}
	if _, err := store.CreateVariant(ctx, domain.Variant{
		SkuID: "sku-2", SKU: "GUC_CLASSIC_TAN_M", ProductID: "parent-2",
		ColorCode: "TAN", SizeCode: "M", Status: "active",
	}); err != nil {
		t.Fatalf("create variant: %v", err)
	}

	got, err := service.NewStorePromotionCatalog(store).
		LineAttributes(ctx, []ports.LineRef{{SkuID: "sku-2", SKU: "GUC_CLASSIC_TAN_M"}})
	if err != nil {
		t.Fatalf("line attributes: %v", err)
	}
	if got[0].OnSale {
		t.Fatal("officialPrice equal to price is not a markdown")
	}
}
