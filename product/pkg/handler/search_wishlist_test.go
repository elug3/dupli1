package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/handler"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
)

func TestSearchProductsSortAndOrder(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "A", Name: "Alpha", Category: "bags", Status: "active", ViewCount: 1, SoldCount: 9, WishlistCount: 2, CreatedAt: "2026-01-01T00:00:00Z"},
		{ID: "B", Name: "Beta", Category: "bags", Status: "active", ViewCount: 10, SoldCount: 1, WishlistCount: 5, CreatedAt: "2026-02-01T00:00:00Z"},
		{ID: "C", Name: "Gamma", Category: "bags", Status: "active", ViewCount: 5, SoldCount: 5, WishlistCount: 1, CreatedAt: "2026-03-01T00:00:00Z"},
	}
	for _, p := range store.Products {
		store.Variants = append(store.Variants, domain.Variant{
			SKU: p.ID + "-SK", ProductID: p.ID, Status: "active", Price: 100,
		})
	}
	mux := newMux(store)

	assertIDs := func(t *testing.T, path string, want ...string) {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d: %s", path, rec.Code, rec.Body.String())
		}
		var resp handler.SearchResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(resp.Results)
		var products []domain.Product
		if err := json.Unmarshal(raw, &products); err != nil {
			t.Fatal(err)
		}
		if len(products) != len(want) {
			t.Fatalf("%s: got %d products, want %d", path, len(products), len(want))
		}
		for i, id := range want {
			if products[i].ID != id {
				t.Fatalf("%s: results[%d]=%s, want %s (got %+v)", path, i, products[i].ID, id, products)
			}
		}
	}

	assertIDs(t, handler.RouteProducts+"?category=bags&sort=views&order=desc", "B", "C", "A")
	assertIDs(t, handler.RouteProducts+"?category=bags&sort=views&order=asc", "A", "C", "B")
	assertIDs(t, handler.RouteProducts+"?category=bags&sort=sold", "A", "C", "B")
	assertIDs(t, handler.RouteProducts+"?category=bags&sort=wishlist", "B", "A", "C")
	assertIDs(t, handler.RouteProducts+"?category=bags&sort=newest", "C", "B", "A")
	assertIDs(t, handler.RouteProducts+"?category=bags&sort=name&order=asc", "A", "B", "C")
	assertIDs(t, handler.RouteProducts+"?category=bags&sort=popular", "B", "C", "A") // alias

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, handler.RouteProducts+"?sort=nope", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid sort: want 400, got %d", rec.Code)
	}
}

func TestSearchProductsTextQuery(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "1", Name: "Cassette", Brand: "Bottega", Description: "intrecciato", Category: "bags", Status: "active"},
		{ID: "2", Name: "Jackie", Brand: "Gucci", Description: "canvas", Category: "bags", Status: "active"},
	}
	mux := newMux(store)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, handler.RouteProducts+"?q=bottega", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp handler.SearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 {
		t.Fatalf("total=%d, want 1", resp.Total)
	}
}

func TestWishlistAddRemoveAndSort(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "HOT", Name: "Hot", Category: "bags", Status: "active"},
		{ID: "COLD", Name: "Cold", Category: "bags", Status: "active"},
	}
	for _, p := range store.Products {
		store.Variants = append(store.Variants, domain.Variant{SKU: p.ID, ProductID: p.ID, Status: "active", Price: 10})
	}
	mux := newMux(store)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/products/HOT/wishlist", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("add wishlist: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	cookie := rec.Result().Header.Get("Set-Cookie")
	if cookie == "" {
		t.Fatal("expected guest cookie on first wishlist")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, handler.RouteProducts+"?sort=wishlist", nil)
	req.Header.Set("Cookie", cookie)
	mux.ServeHTTP(rec, req)
	var resp handler.SearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(resp.Results)
	var products []domain.Product
	json.Unmarshal(raw, &products)
	if len(products) < 1 || products[0].ID != "HOT" || products[0].WishlistCount != 1 {
		t.Fatalf("want HOT first with wishlistCount=1, got %+v", products)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, handler.RouteWishlist, nil)
	req.Header.Set("Cookie", cookie)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list wishlist: want 200, got %d", rec.Code)
	}
	var list struct {
		Items []domain.Product `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].ID != "HOT" {
		t.Fatalf("wishlist items=%+v", list.Items)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/products/HOT/wishlist", nil)
	req.Header.Set("Cookie", cookie)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove wishlist: want 200, got %d", rec.Code)
	}

	got, _ := store.GetProduct("HOT")
	if got.WishlistCount != 0 {
		t.Fatalf("after remove wishlistCount=%d, want 0", got.WishlistCount)
	}
}
