package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/handler"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
)

func TestCatalogBrandCRUD(t *testing.T) {
	mux, _ := newFullMux(memory.NewProductStore())

	body, _ := json.Marshal(map[string]string{"code": "ZZ", "name": "Zegna"})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, handler.RouteCatalogBrands, bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create brand: want 201, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, handler.RouteCatalogBrands, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list brands: want 200, got %d", rec.Code)
	}
	var brands []domain.Brand
	json.NewDecoder(rec.Body).Decode(&brands)
	found := false
	for _, b := range brands {
		if b.Code == "ZZ" && b.Name == "Zegna" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ZZ not in list: %+v", brands)
	}

	patch, _ := json.Marshal(map[string]string{"name": "Ermenegildo Zegna"})
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/catalog/brands/ZZ", bytes.NewReader(patch))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	styleBody, _ := json.Marshal(map[string]string{"code": "STY1", "name": "Test Style"})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/catalog/brands/ZZ/styles", bytes.NewReader(styleBody)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create style: want 201, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/catalog/brands/ZZ", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete brand with styles: want 409, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/catalog/brands/ZZ/styles/STY1", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete style: want 204, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/catalog/brands/ZZ", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete brand: want 204, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCatalogColorRenameAndDelete(t *testing.T) {
	mux, _ := newFullMux(memory.NewProductStore())

	body, _ := json.Marshal(map[string]string{"code": "F9999", "name": "Special Red"})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, handler.RouteCatalogColors, bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create color: want 201, got %d: %s", rec.Code, rec.Body.String())
	}

	patch, _ := json.Marshal(map[string]string{"name": "Crimson"})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/catalog/colors/F9999", bytes.NewReader(patch)))
	if rec.Code != http.StatusOK {
		t.Fatalf("rename color: want 200, got %d", rec.Code)
	}
	var c domain.Color
	json.NewDecoder(rec.Body).Decode(&c)
	if c.Name != "Crimson" {
		t.Fatalf("want Crimson, got %q", c.Name)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/catalog/colors/F9999", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete color: want 204, got %d", rec.Code)
	}
}
