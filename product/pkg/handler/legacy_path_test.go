package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/handler"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
)

func TestAdminCreateViaLegacyAndCurrentPaths(t *testing.T) {
	mux := newAccessControlMux(memory.NewProductStore())
	token := makeAccessToken(t, "admin-1", []string{"admin"})
	body := domain.Product{Name: "Tote", Brand: "Baggu", Price: 45}

	w := serve(t, mux, http.MethodPost, handler.RouteProducts, token, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/products: status=%d, want 201; body=%s", w.Code, w.Body.String())
	}
}

func TestAdminReadLegacyAllPathWithToken(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BAG-001", Name: "Canvas Tote", Brand: "Baggu", Status: "active"},
	}
	mux := newAccessControlMux(store)
	token := makeAccessToken(t, "admin-1", []string{"admin"})

	w := serve(t, mux, http.MethodGet, handler.RouteProductsAll, token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/products/all with admin token: status=%d, want 200; body=%s", w.Code, w.Body.String())
	}

	var resp handler.SearchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Total != 1 {
		t.Fatalf("total = %d, want 1", resp.Total)
	}
}

func TestLegacyAllPathRejectsUnauthenticated(t *testing.T) {
	mux := newAccessControlMux(memory.NewProductStore())
	w := serve(t, mux, http.MethodGet, handler.RouteProductsAll, "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/v1/products/all without auth: status=%d, want 401", w.Code)
	}
}
