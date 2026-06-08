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

func TestSearchBags(t *testing.T) {
	store := memory.NewProductStore()
	store.Bags = []domain.Bag{
		{Product: domain.Product{ID: "1", Name: "Canvas Tote", Brand: "Baggu", Price: 45.0}},
		{Product: domain.Product{ID: "2", Name: "Leather Backpack", Brand: "Herschel", Price: 120.0}},
	}
	mux := newMux(store)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/products/bags", nil))

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
