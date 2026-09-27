package pg

import (
	"errors"
	"testing"
	"time"

	"github.com/elug3/dupli1/product/pkg/ports"
)

// Every variant has a stock row, and stock_items restricts deleting the
// variant it points at — so a variant was undeletable even at zero stock.
// An empty row now goes with the variant; a non-empty one still refuses.
func TestDeleteVariant_DropsEmptyStockRowOnly(t *testing.T) {
	dsn := requireProductDSN(t)
	pool := freshProductSchema(t, dsn, "variant_delete_test")
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

	const sku = "DUP_ECO01_BLK_OS"
	v, err := store.GetVariant(ctx, sku)
	if err != nil {
		t.Fatalf("get variant: %v", err)
	}

	if _, err := stock.SetQuantity(ctx, v.SkuID, 2, time.Now()); err != nil {
		t.Fatalf("set quantity: %v", err)
	}
	if err := store.DeleteVariant(ctx, sku); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("stock on hand: want conflict, got %v", err)
	}
	if _, err := store.GetVariant(ctx, sku); err != nil {
		t.Fatalf("variant must survive a refused delete: %v", err)
	}

	if _, err := stock.SetQuantity(ctx, v.SkuID, 0, time.Now()); err != nil {
		t.Fatalf("zero quantity: %v", err)
	}
	if err := store.DeleteVariant(ctx, sku); err != nil {
		t.Fatalf("empty stock row: want delete, got %v", err)
	}
	if _, err := store.GetVariant(ctx, sku); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("variant still present after delete: %v", err)
	}
	if _, err := stock.GetItem(ctx, v.SkuID); !errors.Is(err, ports.ErrInventoryItemNotFound) {
		t.Fatalf("stock row should go with the variant: %v", err)
	}
}
