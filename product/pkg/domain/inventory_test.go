package domain

import "testing"

func TestStockItem_WithoutStockAvailable(t *testing.T) {
	item := StockItem{Quantity: QuantityWithoutStock, Reserved: 4}
	if !item.IsWithoutStock() {
		t.Fatal("want IsWithoutStock")
	}
	if item.Available() != QuantityWithoutStock {
		t.Fatalf("Available() = %d, want %d", item.Available(), QuantityWithoutStock)
	}
}

func TestStockItem_ZeroIsOutOfStock(t *testing.T) {
	item := StockItem{Quantity: 0, Reserved: 0}
	if item.IsWithoutStock() {
		t.Fatal("quantity 0 is OOS, not without-stock")
	}
	if item.Available() != 0 {
		t.Fatalf("Available() = %d, want 0", item.Available())
	}
}
