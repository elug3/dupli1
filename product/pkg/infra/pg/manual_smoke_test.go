package pg

import (
	"context"
	"testing"
)

// Temporary manual verification test — not part of the permanent suite.
func TestManualViewCountSmoke(t *testing.T) {
	store, err := NewProductStore("postgres://dupli1:dupli1_dev@localhost:5433/products?sslmode=disable")
	if err != nil {
		t.Skipf("no local product postgres available: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	id := "SMK-VIEWCOUNT-001"
	store.pool.Exec(ctx, `DELETE FROM products WHERE id = $1`, id)
	_, err = store.pool.Exec(ctx, `INSERT INTO products (id, name, status) VALUES ($1, 'smoke', 'active')`, id)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	defer store.pool.Exec(ctx, `DELETE FROM products WHERE id = $1`, id)

	got1, err := store.RecordProductView(id, "1.2.3.4", "2024-01-01")
	if err != nil {
		t.Fatalf("first RecordProductView: %v", err)
	}
	if !got1 {
		t.Fatalf("want first view to increment")
	}

	got2, err := store.RecordProductView(id, "1.2.3.4", "2024-01-01")
	if err != nil {
		t.Fatalf("second RecordProductView: %v", err)
	}
	if got2 {
		t.Fatalf("want same-day repeat to be deduped")
	}

	got3, err := store.RecordProductView(id, "5.6.7.8", "2024-01-01")
	if err != nil {
		t.Fatalf("third RecordProductView: %v", err)
	}
	if !got3 {
		t.Fatalf("want different visitor to increment")
	}

	prod, err := store.GetProduct(id)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if prod.ViewCount != 2 {
		t.Fatalf("want ViewCount=2, got %d", prod.ViewCount)
	}
	t.Logf("smoke test OK: ViewCount=%d", prod.ViewCount)
}
