package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
)

// Prices are whole won (money.Won). v1 bodies keep accepting a whole amount
// in any JSON number form, and refuse a fractional or negative one with the
// 400 a variant override has always had, now for the parent price too.
func TestV1PriceInput(t *testing.T) {
	newPriceMux := func(t *testing.T) *http.ServeMux {
		store := memory.NewProductStore()
		seedCatalogStyle(t, store.Catalog, "BOT", "CAS001", "Cassette")
		store.Products = []domain.Product{{
			ID: "BOT-001", Name: "Cassette", BrandCode: "BOT", StyleCode: "CAS001",
			Category: "bags", Status: "active", Price: 300000,
		}}
		store.Variants = []domain.Variant{{
			SKU: "BOT_CAS001_GRN_OS", ProductID: "BOT-001", Color: "Green", ColorCode: "GRN", SizeCode: "OS", Status: "active",
		}}
		mux, _ := newFullMux(store)
		return mux
	}
	const product = "/api/v1/products/BOT-001"
	const variant = "/api/v1/products/BOT-001/variants/BOT_CAS001_GRN_OS"

	for _, tc := range []struct {
		name, method, path, body string
		status                   int
		want                     string // substring of the response body
	}{
		{"whole float notation", http.MethodPut, product, `{"price": 1000000.0, "officialPrice": 1.2e6}`,
			http.StatusOK, `"officialPrice":1200000,"price":1000000,`},
		{"fractional price", http.MethodPut, product, `{"price": 1234.5}`,
			http.StatusBadRequest, `{"error":"invalid request: price must be a non-negative whole won amount","code":400}`},
		{"negative price", http.MethodPut, product, `{"price": -5}`,
			http.StatusBadRequest, `"invalid request: price must be a non-negative whole won amount"`},
		{"fractional official price", http.MethodPut, product, `{"officialPrice": 1300000.25}`,
			http.StatusBadRequest, `"invalid request: officialPrice must be a non-negative whole won amount"`},
		{"price as a string", http.MethodPut, product, `{"price": "1000"}`,
			http.StatusBadRequest, `{"error":"invalid request body","code":400}`},
		{"fractional override", http.MethodPost, "/api/v1/products/BOT-001/variants", `{"colorCode": "BLK", "sizeCode": "OS", "priceOverride": 10.5}`,
			http.StatusBadRequest, `{"error":"invalid request: priceOverride must be a non-negative whole won amount","code":400}`},
		{"negative override", http.MethodPost, "/api/v1/products/BOT-001/variants", `{"colorCode": "BLK", "sizeCode": "OS", "priceOverride": -1}`,
			http.StatusBadRequest, `{"error":"invalid request: priceOverride must be a non-negative whole won amount","code":400}`},
		{"fractional official override", http.MethodPut, variant, `{"officialPriceOverride": 1100000.5}`,
			http.StatusBadRequest, `"invalid request: officialPriceOverride must be a non-negative whole won amount"`},
		{"override in exponent form", http.MethodPut, variant, `{"priceOverride": 3.5e5}`,
			http.StatusOK, `"price":350000,"priceOverride":350000,`},
		{"inline variant negative override", http.MethodPost, "/api/v1/products",
			`{"name": "Inline", "brandCode": "BOT", "styleCode": "CAS001", "category": "bags", "price": 5e5, "variants": [{"colorCode": "RED", "sizeCode": "OS", "priceOverride": -7}]}`,
			http.StatusBadRequest, `"invalid request: variants[0]: priceOverride must be a non-negative whole won amount"`},
		{"inline variant fractional override", http.MethodPost, "/api/v1/products",
			`{"name": "Inline", "brandCode": "BOT", "styleCode": "CAS001", "category": "bags", "price": 5e5, "variants": [{"colorCode": "RED", "sizeCode": "OS", "priceOverride": 7.5}]}`,
			http.StatusBadRequest, `"invalid request: variants.priceOverride must be a non-negative whole won amount"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newPriceMux(t).ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
			if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("%s %s %s\n got %d %s\nwant %d containing %s", tc.method, tc.path, tc.body,
					rec.Code, rec.Body.String(), tc.status, tc.want)
			}
		})
	}
}
