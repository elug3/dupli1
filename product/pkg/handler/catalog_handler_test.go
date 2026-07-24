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

func TestCatalogSubcategoryOccasionTargetSeedsAndCRUD(t *testing.T) {
	mux, _ := newFullMux(memory.NewProductStore())

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, handler.RouteCatalogSubcategories, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list subcategories: want 200, got %d", rec.Code)
	}
	var subs []domain.Subcategory
	json.NewDecoder(rec.Body).Decode(&subs)
	found := map[string]bool{}
	for _, s := range subs {
		found[s.Code] = true
	}
	for _, code := range []string{"HBG", "TOT", "SHD", "CRS", "MNI"} {
		if !found[code] {
			t.Fatalf("missing seeded subcategory %s in %+v", code, subs)
		}
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, handler.RouteCatalogOccasions, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list occasions: want 200, got %d", rec.Code)
	}
	var occasions []domain.Occasion
	json.NewDecoder(rec.Body).Decode(&occasions)
	found = map[string]bool{}
	for _, o := range occasions {
		found[o.Code] = true
	}
	for _, code := range []string{"CAS", "EVE", "BUS", "WKD", "STM"} {
		if !found[code] {
			t.Fatalf("missing seeded occasion %s in %+v", code, occasions)
		}
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, handler.RouteCatalogTargets, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list targets: want 200, got %d", rec.Code)
	}
	var targets []domain.Target
	json.NewDecoder(rec.Body).Decode(&targets)
	found = map[string]bool{}
	for _, tg := range targets {
		found[tg.Code] = true
	}
	for _, code := range []string{"MEN", "WMN", "KID"} {
		if !found[code] {
			t.Fatalf("missing seeded target %s in %+v", code, targets)
		}
	}

	body, _ := json.Marshal(map[string]string{"code": "CLH", "name": "Clutch"})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, handler.RouteCatalogSubcategories, bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create subcategory: want 201, got %d: %s", rec.Code, rec.Body.String())
	}

	patch, _ := json.Marshal(map[string]string{"name": "Evening Clutch"})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/catalog/subcategories/CLH", bytes.NewReader(patch)))
	if rec.Code != http.StatusOK {
		t.Fatalf("rename subcategory: want 200, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/catalog/subcategories/CLH", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete subcategory: want 204, got %d", rec.Code)
	}
}
