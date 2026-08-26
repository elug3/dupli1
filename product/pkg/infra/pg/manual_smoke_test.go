//go:build manual

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
	store.pool.Exec(ctx, `DELETE FROM product_views WHERE product_id = $1`, id)
	store.pool.Exec(ctx, `DELETE FROM products WHERE id = $1`, id)
	_, err = store.pool.Exec(ctx, `INSERT INTO products (id, name, status) VALUES ($1, 'smoke', 'active')`, id)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	defer store.pool.Exec(ctx, `DELETE FROM product_views WHERE product_id = $1`, id)
	defer store.pool.Exec(ctx, `DELETE FROM products WHERE id = $1`, id)

	got1, _, err := store.RecordUniqueView("1.2.3.4", id)
	if err != nil {
		t.Fatalf("first RecordUniqueView: %v", err)
	}
	if !got1 {
		t.Fatalf("want first view to increment")
	}

	got2, _, err := store.RecordUniqueView("1.2.3.4", id)
	if err != nil {
		t.Fatalf("second RecordUniqueView: %v", err)
	}
	if got2 {
		t.Fatalf("want same guest repeat to be deduped")
	}

	got3, _, err := store.RecordUniqueView("5.6.7.8", id)
	if err != nil {
		t.Fatalf("third RecordUniqueView: %v", err)
	}
	if !got3 {
		t.Fatalf("want different guest to increment")
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
