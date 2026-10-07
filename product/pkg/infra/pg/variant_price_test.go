package pg

import (
	"testing"
)

// A SKU override is stored and read back as nil (inherit) or a value, and
// clearing it returns the SKU to inheriting the parent's price.
func TestVariantPriceOverride_RoundTrip(t *testing.T) {
	dsn := requireProductDSN(t)
	pool := freshProductSchema(t, dsn, "variant_price_test")
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
		t.Fatalf("seed: %v", err)
	}

	v, err := store.GetVariant(ctx, "DUP_ECO01_BLK_OS")
	if err != nil {
		t.Fatalf("get variant: %v", err)
	}
	if v.PriceOverride != nil || v.OfficialPriceOverride != nil {
		t.Fatalf("fresh variant must inherit, got %v/%v", v.PriceOverride, v.OfficialPriceOverride)
	}

	own, official := 380000.0, 450000.0
	v.PriceOverride, v.OfficialPriceOverride = &own, &official
	if _, err := store.UpdateVariant(ctx, *v); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := store.GetVariant(ctx, v.SKU)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.PriceOverride == nil || *got.PriceOverride != own ||
		got.OfficialPriceOverride == nil || *got.OfficialPriceOverride != official {
		t.Fatalf("override not persisted: %v/%v", got.PriceOverride, got.OfficialPriceOverride)
	}

	parent, err := store.GetProduct(ctx, v.ProductID)
	if err != nil {
		t.Fatalf("get parent: %v", err)
	}
	got.ApplyParentPrice(*parent)
	if got.Price != own {
		t.Fatalf("effective price = %v, want override %v", got.Price, own)
	}

	got.PriceOverride, got.OfficialPriceOverride = nil, nil
	if _, err := store.UpdateVariant(ctx, *got); err != nil {
		t.Fatalf("clear: %v", err)
	}
	cleared, _ := store.GetVariant(ctx, v.SKU)
	cleared.ApplyParentPrice(*parent)
	if cleared.PriceOverride != nil || cleared.Price != parent.Price {
		t.Fatalf("cleared SKU must inherit parent %v, got price=%v override=%v", parent.Price, cleared.Price, cleared.PriceOverride)
	}
}
