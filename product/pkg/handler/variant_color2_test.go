package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
)

// A two-color sku keeps its one-color sku code and names its second color
// with color2Code: reads carry both swatch fills, the product lists both
// base colors, search finds it by either, and the second color can be
// changed or cleared.
func TestVariantSecondColor(t *testing.T) {
	store := memory.NewProductStore()
	seedCatalogStyle(t, store.Catalog, "BOT", "CAS001", "Cassette")
	if _, err := store.Catalog.CreateColor(t.Context(), domain.Color{Code: "1081", Name: "블랙/브라운", BaseCode: "BLK"}); err != nil {
		t.Fatal(err)
	}
	store.Products = []domain.Product{{ID: "BOT-001", Name: "Cassette", BrandCode: "BOT", StyleCode: "CAS001", Status: "active"}}
	mux, _ := newFullMux(store)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}
	variant := func(rec *httptest.ResponseRecorder) domain.Variant {
		t.Helper()
		var v domain.Variant
		json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&v)
		return v
	}
	const path = "/api/v1/products/BOT-001/variants"

	if rec := do(http.MethodPost, path, `{"colorCode":"1081","color2Code":"1081"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("second color = first: want 400, got %d %s", rec.Code, rec.Body.String())
	}
	rec := do(http.MethodPost, path, `{"colorCode":"1081","color2Code":"brn"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	if v := variant(rec); v.SKU != "BOT_CAS001_1081_OS" || v.Color2Code != "BRN" {
		t.Fatalf("created: sku %q color2Code %q", v.SKU, v.Color2Code)
	}
	sku := path + "/BOT_CAS001_1081_OS"

	rec = do(http.MethodGet, "/api/v1/products/BOT-001", "")
	var p domain.Product
	json.NewDecoder(rec.Body).Decode(&p)
	if len(p.Variants) != 1 {
		t.Fatalf("variants: %+v", p.Variants)
	}
	if v := p.Variants[0]; v.BaseColorCode != "BLK" || v.BaseColor2Code != "BRN" || v.ColorHex != "#0a0a0a" || v.ColorHex2 != "#6b4226" {
		t.Fatalf("PDP swatch: %+v", v)
	}
	if got := strings.Join(p.AvailableBaseColors, ","); got != "BLK,BRN" {
		t.Fatalf("availableBaseColors: %s", got)
	}
	rec = do(http.MethodGet, "/api/v1/products?baseColor=brn", "")
	if !strings.Contains(rec.Body.String(), `"BOT-001"`) {
		t.Fatalf("search by second color's base: %s", rec.Body.String())
	}

	for name, tc := range map[string]struct {
		body string
		want int
	}{
		"same as first color": {`{"color2Code":"1081"}`, http.StatusBadRequest},
		"unknown color":       {`{"color2Code":"ZZZ"}`, http.StatusNotFound},
		"bad code":            {`{"color2Code":"!!!"}`, http.StatusBadRequest},
	} {
		if rec := do(http.MethodPut, sku, tc.body); rec.Code != tc.want {
			t.Errorf("%s: want %d, got %d %s", name, tc.want, rec.Code, rec.Body.String())
		}
	}
	if rec := do(http.MethodDelete, "/api/v1/catalog/colors/BRN", ""); rec.Code != http.StatusConflict {
		t.Fatalf("delete a second color in use: want 409, got %d", rec.Code)
	}

	// Omitted keeps it; a new code replaces it; "" clears it. The sku never changes.
	if v := variant(do(http.MethodPut, sku, `{"status":"active"}`)); v.Color2Code != "BRN" {
		t.Fatalf("omitted color2Code: %+v", v)
	}
	if v := variant(do(http.MethodPut, sku, `{"color2Code":"tan"}`)); v.Color2Code != "TAN" || v.SKU != "BOT_CAS001_1081_OS" {
		t.Fatalf("change color2Code: %+v", v)
	}
	rec = do(http.MethodPut, sku, `{"color2Code":""}`)
	if v := variant(rec); rec.Code != http.StatusOK || v.Color2Code != "" {
		t.Fatalf("clear color2Code: %d %+v", rec.Code, v)
	}
	rec = do(http.MethodGet, "/api/v1/products/BOT-001", "")
	p = domain.Product{} // Decode keeps fields the new body omits
	json.NewDecoder(rec.Body).Decode(&p)
	if v := p.Variants[0]; v.Color2Code != "" || v.ColorHex2 != "" || v.BaseColor2Code != "" {
		t.Fatalf("after clear: %+v", v)
	}
	if rec := do(http.MethodDelete, "/api/v1/catalog/colors/BRN", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete a color no longer used: want 204, got %d %s", rec.Code, rec.Body.String())
	}
}
