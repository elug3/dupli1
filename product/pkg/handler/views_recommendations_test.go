package handler_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/handler"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/service"
)

type failingViewStore struct{}

func (failingViewStore) RecordUniqueView(guestID, productID string) (bool, error) {
	return false, errors.New("view store down")
}

func TestPublicGetProductUniqueViewCookie(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BOT-001", Name: "Mini Bag", Brand: "Bottega", Status: "active", Category: "bags"},
	}
	store.Variants = []domain.Variant{
		{SKU: "BOT-001", ProductID: "BOT-001", Color: "Green", Price: 100, Status: "active"},
	}
	mux := newMux(store)

	rec1 := httptest.NewRecorder()
	mux.ServeHTTP(rec1, httptest.NewRequest(http.MethodGet, "/api/v1/products/BOT-001", nil))
	if rec1.Code != http.StatusOK {
		t.Fatalf("first GET: want 200, got %d: %s", rec1.Code, rec1.Body.String())
	}
	cookies := rec1.Result().Cookies()
	var guest *http.Cookie
	for _, c := range cookies {
		if c.Name == "dupli1_guest" {
			guest = c
			break
		}
	}
	if guest == nil || guest.Value == "" {
		t.Fatal("expected Set-Cookie dupli1_guest on first PDP")
	}
	var p1 domain.Product
	if err := json.NewDecoder(rec1.Body).Decode(&p1); err != nil {
		t.Fatal(err)
	}
	if p1.ViewCount != 1 {
		t.Fatalf("first view: want viewCount=1, got %d", p1.ViewCount)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/products/BOT-001", nil)
	req2.AddCookie(guest)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second GET: want 200, got %d", rec2.Code)
	}
	var p2 domain.Product
	if err := json.NewDecoder(rec2.Body).Decode(&p2); err != nil {
		t.Fatal(err)
	}
	if p2.ViewCount != 1 {
		t.Fatalf("reload: want viewCount=1, got %d", p2.ViewCount)
	}

	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/products/BOT-001", nil)
	rec3 := httptest.NewRecorder()
	mux.ServeHTTP(rec3, req3)
	var p3 domain.Product
	if err := json.NewDecoder(rec3.Body).Decode(&p3); err != nil {
		t.Fatal(err)
	}
	if p3.ViewCount != 2 {
		t.Fatalf("different guest: want viewCount=2, got %d", p3.ViewCount)
	}
}

func TestPublicGetProductViewFailureStillOK(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BOT-001", Name: "Mini Bag", Status: "active"},
	}
	store.Variants = []domain.Variant{
		{SKU: "BOT-001", ProductID: "BOT-001", Price: 100, Status: "active"},
	}
	svc := service.NewProductSearchService(store, nil)
	h := handler.NewHandler(svc, service.NewCouponService(memory.NewCouponStore()), nil, service.NewCatalogService(store.Catalog)).
		WithViewStore(failingViewStore{})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/products/BOT-001", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 despite view failure, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPublicGetRecommendations(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "SEED", Name: "Seed", BrandCode: "BOT", Brand: "Bottega", Material: "leather", Category: "bags", Status: "active", Tags: []string{"tote"}, PriceFrom: 200},
		{ID: "BEST", Name: "Best", BrandCode: "BOT", Brand: "Bottega", Material: "leather", Category: "bags", Status: "active", Tags: []string{"tote"}, PriceFrom: 210, ViewCount: 5},
		{ID: "OK", Name: "Ok", BrandCode: "BOT", Brand: "Bottega", Category: "bags", Status: "active", PriceFrom: 200, ViewCount: 20},
		{ID: "OTHER", Name: "Other", Brand: "Gucci", Category: "bags", Status: "active", ViewCount: 999},
		{ID: "DRAFT", Name: "Draft", BrandCode: "BOT", Category: "bags", Status: "draft"},
		{ID: "SHOES", Name: "Shoes", BrandCode: "BOT", Category: "shoes", Status: "active"},
	}
	for _, p := range store.Products {
		if p.Status == "active" {
			store.Variants = append(store.Variants, domain.Variant{
				SKU: p.ID, ProductID: p.ID, Price: p.PriceFrom, Status: "active",
			})
		}
	}
	mux := newMux(store)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/products/SEED/recommendations?limit=3", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp handler.RecommendationsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.SeedID != "SEED" {
		t.Fatalf("seedId: want SEED, got %s", resp.SeedID)
	}
	if len(resp.Items) != 3 {
		t.Fatalf("want 3 items, got %d", len(resp.Items))
	}
	if resp.Items[0].ID != "BEST" {
		t.Fatalf("want BEST first, got %s", resp.Items[0].ID)
	}
	for _, item := range resp.Items {
		if item.ID == "SEED" || item.ID == "DRAFT" || item.ID == "SHOES" {
			t.Fatalf("unexpected item %s", item.ID)
		}
	}

	rec404 := httptest.NewRecorder()
	mux.ServeHTTP(rec404, httptest.NewRequest(http.MethodGet, "/api/v1/products/MISSING/recommendations", nil))
	if rec404.Code != http.StatusNotFound {
		t.Fatalf("missing seed: want 404, got %d", rec404.Code)
	}
}
