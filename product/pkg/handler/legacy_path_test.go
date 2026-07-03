package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/handler"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
)

func TestAdminListsProductsAtDocumentedPath(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BAG-001", Name: "Canvas Tote", Brand: "Baggu", Status: "active", Cost: 20},
	}
	mux := newAccessControlMux(store)
	token := makeAccessToken(t, "admin-1", []string{"admin"})

	w := serve(t, mux, http.MethodGet, handler.RouteProducts, token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/products: status=%d, want 200; body=%s", w.Code, w.Body.String())
	}

	var products []domain.Product
	if err := json.NewDecoder(w.Body).Decode(&products); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(products) != 1 {
		t.Fatalf("len(products) = %d, want 1", len(products))
	}
	if products[0].Cost != 20 {
		t.Fatalf("cost = %v, want admin list to include cost", products[0].Cost)
	}
}

func TestAdminCreatesProductAtDocumentedPath(t *testing.T) {
	mux := newAccessControlMux(memory.NewProductStore())
	token := makeAccessToken(t, "admin-1", []string{"admin"})
	body := domain.Product{Name: "Tote", Brand: "Baggu", Price: 45}

	w := serve(t, mux, http.MethodPost, handler.RouteProducts, token, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/products: status=%d, want 201; body=%s", w.Code, w.Body.String())
	}
}

func TestProductsAllIsNotAdminListEndpoint(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BAG-001", Name: "Canvas Tote", Brand: "Baggu", Status: "active"},
	}
	mux := newAccessControlMux(store)
	token := makeAccessToken(t, "admin-1", []string{"admin"})

	// /api/v1/products/all is not documented; it falls through to the public PDP route.
	w := serve(t, mux, http.MethodGet, "/api/v1/products/all", token, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /api/v1/products/all: status=%d, want 404", w.Code)
	}
}
