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
	if len(catalog.Targets) != 3 || catalog.Targets[0].Code != "men" {
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
