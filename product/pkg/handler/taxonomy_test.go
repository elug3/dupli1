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

func TestMasterCatalogEndpoints(t *testing.T) {
	mux, _ := newFullMux(memory.NewProductStore())

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, handler.RouteCatalogMaster, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("master: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var catalog domain.MasterCatalog
	if err := json.NewDecoder(rec.Body).Decode(&catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.SubCategories) != 5 || catalog.SubCategories[0].Code != "handbags" {
		t.Fatalf("subCategories: %+v", catalog.SubCategories)
	}
	if len(catalog.Styles) != 5 || catalog.Styles[0].Code != "casual" {
		t.Fatalf("styles: %+v", catalog.Styles)
	}
	if len(catalog.Targets) != 4 || catalog.Targets[0].Code != "all" {
		t.Fatalf("targets: %+v", catalog.Targets)
	}

	for _, path := range []string{
		handler.RouteCatalogSubCategories,
		handler.LegacyRouteCatalogBagStyles,
		handler.RouteCatalogTargets,
	} {
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d", path, rec.Code)
		}
	}
}

func TestCategoryScopedCatalogEndpoints(t *testing.T) {
	mux, _ := newFullMux(memory.NewProductStore())

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, handler.RouteCatalogCategories, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("categories: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var cats []domain.Category
	if err := json.NewDecoder(rec.Body).Decode(&cats); err != nil {
		t.Fatal(err)
	}
	if len(cats) != 2 || cats[0].Code != "bags" || cats[1].Code != "clothing" {
		t.Fatalf("categories: %+v", cats)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, handler.RouteCatalogMaster+"?category=clothing", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("clothing master: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var catalog domain.MasterCatalog
	if err := json.NewDecoder(rec.Body).Decode(&catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.SubCategories) != 2 || catalog.SubCategories[0].Code != "padded" || catalog.SubCategories[1].Code != "jackets" {
		t.Fatalf("clothing subCategories: %+v", catalog.SubCategories)
	}
	if len(catalog.Styles) != 5 || len(catalog.Targets) != 4 {
		t.Fatalf("shared terms missing: %+v", catalog)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, handler.RouteCatalogSubCategories+"?category=bags", nil))
	var subs []domain.CatalogTerm
	if err := json.NewDecoder(rec.Body).Decode(&subs); err != nil {
		t.Fatal(err)
	}
	if len(subs) != 5 || subs[0].Code != "handbags" {
		t.Fatalf("bag subcategories: %+v", subs)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, handler.RouteCatalogMaster+"?category=shoes", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown category: want 400, got %d", rec.Code)
	}
}

func TestSearchProductsTaxonomyFilters(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "A", Name: "A", Category: "bags", SubCategory: "tote", Style: "casual", Target: "women", Status: "active"},
		{ID: "B", Name: "B", Category: "bags", SubCategory: "mini", Style: "evening", Target: "women", Status: "active"},
		{ID: "C", Name: "C", Category: "bags", SubCategory: "tote", Style: "business", Target: "men", Status: "active"},
	}
	mux := newMux(store)

	assertIDs := func(url string, want ...string) {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d: %s", url, rec.Code, rec.Body.String())
		}
		var resp handler.SearchResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(resp.Results)
		var products []domain.Product
		if err := json.Unmarshal(raw, &products); err != nil {
			t.Fatal(err)
		}
		if len(products) != len(want) {
			t.Fatalf("%s: got %d results want %d (%v)", url, len(products), len(want), products)
		}
		for i, id := range want {
			if products[i].ID != id {
				t.Fatalf("%s: result[%d]=%s want %s", url, i, products[i].ID, id)
			}
		}
	}

	assertIDs(handler.RouteProducts+"?category=bags&subcategory=tote", "A", "C")
	assertIDs(handler.RouteProducts+"?subCategory=mini&style=evening", "B")
	assertIDs(handler.RouteProducts+"?target=men&style=Business", "C")
	assertIDs(handler.RouteProducts+"?target=mem", "C") // typo normalized
}

func TestCreateProductTaxonomyFields(t *testing.T) {
	store := memory.NewProductStore()
	seedCatalogStyle(t, store.Catalog, "BOT", "CAS001", "Cassette")
	mux, _ := newFullMux(store)

	body, _ := json.Marshal(map[string]any{
		"name":        "Cassette",
		"brand":       "Bottega Veneta",
		"brandCode":   "BOT",
		"styleCode":   "CAS001",
		"category":    "bags",
		"subCategory": "Tote",
		"style":       "weekend",
		"target":      "Women",
		"price":       100,
		"color":       "Black",
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, handler.RouteProducts, bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created domain.Product
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.SubCategory != "tote" || created.Style != "weekend" || created.Target != "women" {
		t.Fatalf("taxonomy not normalized: %+v", created)
	}

	bad, _ := json.Marshal(map[string]any{
		"name": "X", "brandCode": "BOT", "styleCode": "CAS001",
		"subCategory": "backpack",
	})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, handler.RouteProducts, bytes.NewReader(bad)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid subcategory: want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// Staff add, rename, reorder and remove terms; products are checked against
// what the store holds right then.
func TestTaxonomyTermCRUD(t *testing.T) {
	store := memory.NewProductStore()
	seedCatalogStyle(t, store.Catalog, "BOT", "CAS001", "Cassette")
	mux, _ := newFullMux(store)
	do := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, &buf))
		return rec
	}
	createProduct := func(sub, style string) int {
		return do(http.MethodPost, handler.RouteProducts, map[string]any{
			"name": "Cassette", "brandCode": "BOT", "styleCode": "CAS001", "price": 100,
			"category": "bags", "subCategory": sub, "style": style,
		}).Code
	}

	// A new bag style is refused on a product until staff add it.
	if code := createProduct("", "resort"); code != http.StatusBadRequest {
		t.Fatalf("unknown style: want 400, got %d", code)
	}
	if rec := do(http.MethodPost, handler.RouteCatalogBagStyles, map[string]any{"code": "Resort", "name": "Resort", "sortOrder": 9}); rec.Code != http.StatusCreated {
		t.Fatalf("create style: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if code := createProduct("", "RESORT"); code != http.StatusCreated {
		t.Fatalf("added style: want 201, got %d", code)
	}
	if rec := do(http.MethodPost, handler.RouteCatalogBagStyles, map[string]any{"code": "resort", "name": "Again"}); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate: want 409, got %d", rec.Code)
	}

	// A clothing subcategory lists under clothing only, and its code is taken
	// for every category.
	if rec := do(http.MethodPost, handler.RouteCatalogSubCategories, map[string]any{"code": "coats", "name": "Coats", "category": "Clothing"}); rec.Code != http.StatusCreated {
		t.Fatalf("create subcategory: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var subs []domain.CatalogTerm
	json.NewDecoder(do(http.MethodGet, handler.RouteCatalogSubCategories+"?category=clothing", nil).Body).Decode(&subs)
	if len(subs) != 3 || subs[2].Code != "coats" || subs[2].Category != "clothing" {
		t.Fatalf("clothing subcategories: %+v", subs)
	}
	json.NewDecoder(do(http.MethodGet, handler.RouteCatalogSubCategories, nil).Body).Decode(&subs)
	for _, sc := range subs {
		if sc.Code == "coats" {
			t.Fatalf("coats listed under bags: %+v", subs)
		}
	}
	if rec := do(http.MethodPost, handler.RouteCatalogSubCategories, map[string]any{"code": "coats", "name": "Coats", "category": "bags"}); rec.Code != http.StatusConflict {
		t.Fatalf("subcategory code in another category: want 409, got %d", rec.Code)
	}

	// Rename and reorder; the code stays.
	if rec := do(http.MethodPatch, "/api/v1/products/catalog/targets/kids", map[string]any{"name": "Children", "sortOrder": 0}); rec.Code != http.StatusOK {
		t.Fatalf("patch target: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var targets []domain.CatalogTerm
	json.NewDecoder(do(http.MethodGet, handler.RouteCatalogTargets, nil).Body).Decode(&targets)
	if targets[0].Code != "all" || targets[1].Code != "kids" || targets[1].Name != "Children" {
		t.Fatalf("targets after patch (sortOrder ties break by code): %+v", targets)
	}

	// A term a product carries cannot go; an unused one can.
	if rec := do(http.MethodDelete, "/api/v1/products/catalog/bag-styles/resort", nil); rec.Code != http.StatusConflict {
		t.Fatalf("delete style in use: want 409, got %d", rec.Code)
	}
	if rec := do(http.MethodDelete, "/api/v1/products/catalog/subcategories/coats", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete unused subcategory: want 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodDelete, "/api/v1/products/catalog/subcategories/coats", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("delete again: want 404, got %d", rec.Code)
	}

	for _, bad := range []struct {
		method, path string
		body         any
		want         int
	}{
		{http.MethodPost, handler.RouteCatalogBagStyles, map[string]any{"code": "a b", "name": "Spaced"}, http.StatusBadRequest},
		{http.MethodPost, handler.RouteCatalogBagStyles, map[string]any{"code": "noname"}, http.StatusBadRequest},
		{http.MethodPost, handler.RouteCatalogBagStyles, map[string]any{"code": "x", "name": "X", "category": "bags"}, http.StatusBadRequest},
		{http.MethodPost, handler.RouteCatalogSubCategories, map[string]any{"code": "boots", "name": "Boots", "category": "shoes"}, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/products/catalog/targets/kids", map[string]any{}, http.StatusBadRequest},
		{http.MethodPatch, "/api/v1/products/catalog/targets/nobody", map[string]any{"name": "Nobody"}, http.StatusNotFound},
	} {
		if rec := do(bad.method, bad.path, bad.body); rec.Code != bad.want {
			t.Errorf("%s %s %v: want %d, got %d: %s", bad.method, bad.path, bad.body, bad.want, rec.Code, rec.Body.String())
		}
	}
}
