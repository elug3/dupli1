package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

	// The base palette is seeded; a base color can be added.
	rec := do(http.MethodPost, handler.RouteCatalogBaseColors, map[string]string{"code": "ivr", "name": "Ivory", "hex": "F4EEDC"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create base color: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(http.MethodGet, handler.RouteCatalogBaseColors, nil)
	var bases []domain.BaseColor
	json.NewDecoder(rec.Body).Decode(&bases)
	byBase := map[string]domain.BaseColor{}
	for _, b := range bases {
		byBase[b.Code] = b
	}
	if byBase["BLK"].Hex != "#0a0a0a" || byBase["IVR"].Hex != "#f4eedc" {
		t.Fatalf("base colors: %+v", byBase)
	}

	// Brand colorways: two "블랙" stay distinct codes with the same base.
	// Each is one color, with no hex of its own.
	for _, c := range []map[string]string{
		{"code": "1139", "name": "블랙", "baseCode": "BLK"},
		{"code": "1249", "name": "블랙", "baseCode": "blk"},
		{"code": "1081", "name": "브라운", "baseCode": "BRN"},
		{"code": "2934", "name": "앨지"},
	} {
		if rec := do(http.MethodPost, handler.RouteCatalogColors, c); rec.Code != http.StatusCreated {
			t.Fatalf("create colorway %s: %d %s", c["code"], rec.Code, rec.Body.String())
		}
	}
	for name, tc := range map[string]struct {
		method, path string
		body         map[string]string
		want         int
	}{
		// A color master row is one color: no second fill, no second base.
		"hex2 on create":      {http.MethodPost, handler.RouteCatalogColors, map[string]string{"code": "BKB", "name": "x", "hex": "#000000", "hex2": "#111111"}, 400},
		"hex2 on update":      {http.MethodPatch, "/api/v1/products/catalog/colors/1139", map[string]string{"hex2": "#111111"}, 400},
		"base2Code on create": {http.MethodPost, handler.RouteCatalogColors, map[string]string{"code": "1206", "name": "블랙/폰단트", "baseCode": "BLK", "base2Code": "BRN"}, 400},
		"base2Code on update": {http.MethodPatch, "/api/v1/products/catalog/colors/1139", map[string]string{"base2Code": "BRN"}, 400},
		// Older clients (dupli1-manage-web) always send hex2 "": accepted.
		"empty hex2 on create": {http.MethodPost, handler.RouteCatalogColors, map[string]string{"code": "1990", "name": "에스프레소", "hex2": "", "base2Code": ""}, 201},
		"empty hex2 on update": {http.MethodPatch, "/api/v1/products/catalog/colors/1249", map[string]string{"name": "블랙", "hex": "", "hex2": ""}, 200},
		"unknown base":         {http.MethodPatch, "/api/v1/products/catalog/colors/2934", map[string]string{"baseCode": "ZZZ"}, 404},
		"base in use":          {http.MethodDelete, "/api/v1/products/catalog/base-colors/BRN", nil, 409},
		"bad base hex":         {http.MethodPatch, "/api/v1/products/catalog/base-colors/IVR", map[string]string{"hex": "ivory"}, 400},
	} {
		if rec := do(tc.method, tc.path, tc.body); rec.Code != tc.want {
			t.Errorf("%s: want %d, got %d: %s", name, tc.want, rec.Code, rec.Body.String())
		}
	}
	// A refused write leaves the colorway as it was, and colors are never
	// listed with a second color.
	rec = do(http.MethodGet, handler.RouteCatalogColors, nil)
	if body := rec.Body.String(); strings.Contains(body, "hex2") || strings.Contains(body, "base2Code") {
		t.Fatalf("colors list carries a second color: %s", body)
	}
	if strings.Contains(rec.Body.String(), `"BKB"`) || strings.Contains(rec.Body.String(), `"1206"`) {
		t.Fatalf("refused create was stored: %s", rec.Body.String())
	}

	// Assign a base later; "" clears one.
	rec = do(http.MethodPatch, "/api/v1/products/catalog/colors/2934", map[string]string{"baseCode": "GRN"})
	if c := decode(rec); rec.Code != http.StatusOK || c.BaseCode != "GRN" {
		t.Fatalf("assign base: %d %+v", rec.Code, c)
	}

	// Variant reads carry the base color and swatch fill from the base; the
	// product lists its base colors, and search filters by base.
	store.Products = []domain.Product{
		{ID: "BOT-001", Name: "Cassette", BrandCode: "BOT", StyleCode: "CAS001", Status: "active"},
		{ID: "BOT-002", Name: "Other", BrandCode: "BOT", StyleCode: "CAS001", Status: "active"},
	}
	store.Variants = []domain.Variant{
		{SKU: "BOT_CAS001_1139_OS", ProductID: "BOT-001", Color: "블랙", ColorCode: "1139", SizeCode: "OS", Status: "active"},
		{SKU: "BOT_CAS001_1081_OS", ProductID: "BOT-001", Color: "브라운", ColorCode: "1081", SizeCode: "OS", Status: "active"},
		{SKU: "BOT_CAS001_2934_OS", ProductID: "BOT-002", Color: "앨지", ColorCode: "2934", SizeCode: "OS", Status: "active"},
	}
	rec = do(http.MethodGet, "/api/v1/products/BOT-001", nil)
	var p domain.Product
	json.NewDecoder(rec.Body).Decode(&p)
	got := map[string][2]string{}
	for _, v := range p.Variants {
		got[v.ColorCode] = [2]string{v.BaseColorCode, v.ColorHex}
	}
	if got["1139"] != [2]string{"BLK", "#0a0a0a"} || got["1081"] != [2]string{"BRN", "#6b4226"} {
		t.Fatalf("PDP base colors / swatch: %+v", got)
	}
	if strings.Join(p.AvailableBaseColors, ",") != "BLK,BRN" {
		t.Fatalf("availableBaseColors: %v", p.AvailableBaseColors)
	}
	search := func(q string) []string {
		rec := do(http.MethodGet, "/api/v1/products?"+q, nil)
		var body struct {
			Results []domain.Product `json:"results"`
		}
		json.NewDecoder(rec.Body).Decode(&body)
		ids := []string{}
		for _, r := range body.Results {
			ids = append(ids, r.ID)
		}
		return ids
	}
	if ids := search("baseColor=brn"); strings.Join(ids, ",") != "BOT-001" {
		t.Errorf("baseColor=brn: %v", ids)
	}
	if ids := search("base_color=GRN"); strings.Join(ids, ",") != "BOT-002" {
		t.Errorf("base_color=GRN: %v", ids)
	}

	// A colorway's own hex overrides its base color's.
	do(http.MethodPatch, "/api/v1/products/catalog/colors/1139", map[string]string{"hex": "#111111"})
	rec = do(http.MethodGet, "/api/v1/products/BOT-001", nil)
	json.NewDecoder(rec.Body).Decode(&p)
	for _, v := range p.Variants {
		if v.ColorCode == "1139" && v.ColorHex != "#111111" {
			t.Errorf("own hex override: %+v", v)
		}
	}
}
