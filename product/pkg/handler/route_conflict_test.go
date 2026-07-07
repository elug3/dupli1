package handler_test

import (
	"net/http"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/handler"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
)

func TestDocumentedProductRoutesRequireAuth(t *testing.T) {
	mux := newAccessControlMux(memory.NewProductStore())

	w := serve(t, mux, http.MethodPost, handler.RouteProducts, "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("POST /api/v1/products without auth: status=%d, want 401", w.Code)
	}
}

func TestPublicRoutesStayOpen(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BAG-001", Name: "Canvas Tote", Brand: "Baggu", Status: "active"},
	}
	mux := newAccessControlMux(store)

	w := serve(t, mux, http.MethodGet, handler.RouteHealth, "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET health: status=%d, want 200", w.Code)
	}

	w = serve(t, mux, http.MethodGet, handler.RouteProducts, "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET products search: status=%d, want 200", w.Code)
	}

	w = serve(t, mux, http.MethodGet, "/api/v1/products/BAG-001", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET public PDP: status=%d, want 200", w.Code)
	}
}
