package service_test

import (
	"errors"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/elug3/dupli1/product/pkg/service"
)

// Names resolve against the masters as they are now, not the compiled-in
// seeds: a color or brand staff added at runtime works by name.
func TestCreate_ResolvesRuntimeMasterNames(t *testing.T) {
	store := memory.NewProductStore()
	ctx := t.Context()
	if _, err := store.Catalog.CreateBrand(ctx, domain.Brand{Code: "MM", Name: "Maison Margiela"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Catalog.CreateStyle(ctx, domain.Style{BrandCode: "MM", Code: "S001", Name: "5AC"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Catalog.CreateColor(ctx, domain.Color{Code: "IVR", Name: "Ivory"}); err != nil {
		t.Fatal(err)
	}
	svc := service.NewProductSearchService(store, nil)

	p, err := svc.CreateProduct(ctx, domain.Product{Name: "5AC", Brand: "maison margiela", StyleCode: "S001", Price: 100000})
	if err != nil {
		t.Fatalf("create by brand name: %v", err)
	}
	if p.BrandCode != "MM" {
		t.Fatalf("brandCode = %q, want MM from the brand master", p.BrandCode)
	}

	v, err := svc.CreateVariant(ctx, p.ID, domain.Variant{Color: "Ivory"})
	if err != nil {
		t.Fatalf("create by color name: %v", err)
	}
	if v.ColorCode != "IVR" || v.SKU != "MM_S001_IVR_OS" {
		t.Fatalf("variant = %s / %s, want IVR / MM_S001_IVR_OS", v.ColorCode, v.SKU)
	}

	// A seeded color still resolves by name, and an unknown one still fails.
	if _, err := svc.CreateVariant(ctx, p.ID, domain.Variant{Color: "Black"}); err != nil {
		t.Fatalf("seeded color by name: %v", err)
	}
	if _, err := svc.CreateVariant(ctx, p.ID, domain.Variant{Color: "Chartreuse"}); !errors.Is(err, domain.ErrMasterNotFound) {
		t.Fatalf("unknown color: want ErrMasterNotFound, got %v", err)
	}
}

// Every size clothing allows can be used, including Italian fits.
func TestCreateVariant_EveryClothingSize(t *testing.T) {
	store := memory.NewProductStore()
	if _, err := store.Catalog.CreateStyle(t.Context(), domain.Style{BrandCode: "BOT", Code: "PUF001", Name: "Puffer"}); err != nil {
		t.Fatal(err)
	}
	store.Products = []domain.Product{
		{ID: "JKT-1", Name: "Puffer", BrandCode: "BOT", StyleCode: "PUF001", Category: "clothing", SubCategory: "padded", Status: "active", Price: 900000},
	}
	svc := service.NewProductSearchService(store, nil)
	clothing, _ := domain.LookupCategory("clothing")
	for _, size := range clothing.Sizes {
		if _, err := svc.CreateVariant(t.Context(), "JKT-1", domain.Variant{ColorCode: "BLK", SizeCode: size}); err != nil {
			t.Fatalf("size %s: %v", size, err)
		}
	}
}

func TestUpdateVariant_CodesAreImmutable(t *testing.T) {
	store := memory.NewProductStore()
	if _, err := store.Catalog.CreateStyle(t.Context(), domain.Style{BrandCode: "BOT", Code: "CAS001", Name: "Cassette"}); err != nil {
		t.Fatal(err)
	}
	store.Products = []domain.Product{
		{ID: "BAG-1", Name: "Cassette", BrandCode: "BOT", StyleCode: "CAS001", Category: "bags", Status: "active", Price: 2500},
	}
	svc := service.NewProductSearchService(store, nil)
	v, err := svc.CreateVariant(t.Context(), "BAG-1", domain.Variant{ColorCode: "BLK", SizeCode: "MED"})
	if err != nil {
		t.Fatal(err)
	}

	for _, change := range []domain.Variant{{ColorCode: "RED"}, {SizeCode: "LRG"}, {EditionCode: "R"}} {
		if _, err := svc.UpdateVariant(t.Context(), "BAG-1", v.SKU, change); !errors.Is(err, ports.ErrInvalid) {
			t.Fatalf("change %+v: want ErrInvalid, got %v", change, err)
		}
	}
	got, err := store.GetVariant(t.Context(), v.SKU)
	if err != nil {
		t.Fatal(err)
	}
	if got.ColorCode != "BLK" || got.SizeCode != "MED" {
		t.Fatalf("codes changed after refused update: %+v", got)
	}

	// Resending the current codes with another change is fine.
	updated, err := svc.UpdateVariant(t.Context(), "BAG-1", v.SKU, domain.Variant{ColorCode: "BLK", SizeCode: "MED", Status: "draft"})
	if err != nil {
		t.Fatalf("unchanged codes: %v", err)
	}
	if updated.Status != "draft" || updated.SKU != v.SKU {
		t.Fatalf("updated = %+v", updated)
	}
}
