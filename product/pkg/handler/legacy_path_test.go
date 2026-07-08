package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/handler"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

func TestAdminListsProductsAtDocumentedPath(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BAG-001", Name: "Canvas Tote", Brand: "Baggu", Status: "active"},
		{ID: "BAG-002", Name: "Draft Tote", Brand: "Baggu", Status: "draft"},
	}
	mux := newAccessControlMux(store)
	token := makeAccessToken(t, "admin-1", permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin}))

	w := serve(t, mux, http.MethodGet, handler.RouteProducts, token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/products: status=%d, want 200; body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Total   int               `json:"total"`
		Results []domain.Product `json:"results"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Total < 2 {
		t.Fatalf("admin search total = %d, want at least 2 (includes drafts)", resp.Total)
	}
}

func TestAdminCreatesProductAtDocumentedPath(t *testing.T) {
	mux := newAccessControlMux(memory.NewProductStore())
	token := makeAccessToken(t, "admin-1", permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin}))
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
	token := makeAccessToken(t, "admin-1", permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin}))

	w := serve(t, mux, http.MethodGet, "/api/v1/products/all", token, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /api/v1/products/all: status=%d, want 404", w.Code)
	}
}
