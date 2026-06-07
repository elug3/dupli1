package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/schick/pkg/product/domain"
	"github.com/schick/pkg/product/handler"
	"github.com/schick/pkg/product/infra/memory"
	"github.com/schick/pkg/product/service"
)

func newMux(store *memory.ProductStore) *http.ServeMux {
	svc := service.NewProductSearchService(store)
	h := handler.NewHandler(svc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func TestHealth(t *testing.T) {
	mux := newMux(memory.NewProductStore())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp handler.HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "healthy" {
		t.Errorf("want healthy, got %q", resp.Status)
	}
}

func TestHealthMethodNotAllowed(t *testing.T) {
	mux := newMux(memory.NewProductStore())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/health", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
}

func TestGetCategories(t *testing.T) {
	mux := newMux(memory.NewProductStore())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/categories", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp handler.CategoryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Categories) != 6 {
		t.Errorf("want 6 categories, got %d", len(resp.Categories))
	}
}

func TestGetFilters(t *testing.T) {
	mux := newMux(memory.NewProductStore())

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/filters?category=shoes", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["category"] != "shoes" {
		t.Errorf("want category=shoes, got %v", resp["category"])
	}
}

func TestGetFiltersUnknownCategory(t *testing.T) {
	mux := newMux(memory.NewProductStore())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/filters?category=unknown", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestGetFiltersMissingCategory(t *testing.T) {
	mux := newMux(memory.NewProductStore())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/filters", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestSearchShoes(t *testing.T) {
	store := memory.NewProductStore()
	store.Shoes = []domain.Shoes{
		{Product: domain.Product{ID: "1", Name: "Air Max", Brand: "Nike", Price: 120.0}},
		{Product: domain.Product{ID: "2", Name: "Stan Smith", Brand: "Adidas", Price: 90.0}},
	}
	mux := newMux(store)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/products/shoes", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp handler.SearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 2 {
		t.Errorf("want total=2, got %d", resp.Total)
	}
}

func TestSearchGenericMissingCategory(t *testing.T) {
	mux := newMux(memory.NewProductStore())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/products/search", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestSearchGenericUnknownCategory(t *testing.T) {
	mux := newMux(memory.NewProductStore())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/products/search?category=unknown", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
}
