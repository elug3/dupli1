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

func TestCatalogColorSwatchHex(t *testing.T) {
	store := memory.NewProductStore()
	seedCatalogStyle(t, store.Catalog, "BOT", "CAS001", "Cassette")
	mux, _ := newFullMux(store)

	do := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			json.NewEncoder(&buf).Encode(body)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, &buf))
		return rec
	}
	decode := func(rec *httptest.ResponseRecorder) domain.Color {
		t.Helper()
		var c domain.Color
		json.NewDecoder(rec.Body).Decode(&c)
		return c
	}

	rec := do(http.MethodPost, handler.RouteCatalogColors, map[string]string{"code": "BKB", "name": "Black / Brown", "hex": "0A0A0A", "hex2": "#6B4226"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if c := decode(rec); c.Hex != "#0a0a0a" || c.Hex2 != "#6b4226" {
		t.Fatalf("create normalizes hex: got %+v", c)
	}

	// Renaming keeps the hexes.
	rec = do(http.MethodPatch, "/api/v1/products/catalog/colors/BKB", map[string]string{"name": "Noir / Brun"})
	if c := decode(rec); rec.Code != http.StatusOK || c.Hex != "#0a0a0a" || c.Hex2 != "#6b4226" {
		t.Fatalf("rename keeps hex: %d %+v", rec.Code, c)
	}

	for name, body := range map[string]map[string]string{
		"bad hex":          {"hex": "brown"},
		"hex2 without hex": {"hex": ""},
	} {
		if rec := do(http.MethodPatch, "/api/v1/products/catalog/colors/BKB", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d: %s", name, rec.Code, rec.Body.String())
		}
	}

	// "" clears hex2: back to a solid swatch.
	rec = do(http.MethodPatch, "/api/v1/products/catalog/colors/BKB", map[string]string{"hex2": ""})
	if c := decode(rec); rec.Code != http.StatusOK || c.Hex != "#0a0a0a" || c.Hex2 != "" {
		t.Fatalf("clear hex2: %d %+v", rec.Code, c)
	}
	do(http.MethodPatch, "/api/v1/products/catalog/colors/BKB", map[string]string{"hex2": "#6b4226"})

	// The PDP carries each variant's swatch fills.
	store.Products = []domain.Product{{ID: "BOT-001", Name: "Cassette", BrandCode: "BOT", StyleCode: "CAS001", Status: "active"}}
	store.Variants = []domain.Variant{
		{SKU: "BOT_CAS001_BKB_OS", ProductID: "BOT-001", Color: "Black / Brown", ColorCode: "BKB", SizeCode: "OS", Status: "active"},
		{SKU: "BOT_CAS001_BLK_OS", ProductID: "BOT-001", Color: "Black", ColorCode: "BLK", SizeCode: "OS", Status: "active"},
	}
	rec = do(http.MethodGet, "/api/v1/products/BOT-001", nil)
	var p domain.Product
	json.NewDecoder(rec.Body).Decode(&p)
	got := map[string][2]string{}
	for _, v := range p.Variants {
		got[v.ColorCode] = [2]string{v.ColorHex, v.ColorHex2}
	}
	if got["BKB"] != [2]string{"#0a0a0a", "#6b4226"} || got["BLK"] != [2]string{"#0a0a0a", ""} {
		t.Fatalf("PDP swatch hexes: %+v", got)
	}
}
