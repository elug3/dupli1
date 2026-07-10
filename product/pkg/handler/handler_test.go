package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/handler"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/service"
)

func newMux(store *memory.ProductStore) *http.ServeMux {
	svc := service.NewProductSearchService(store, nil)
	couponSvc := service.NewCouponService(memory.NewCouponStore())
	h := handler.NewHandler(svc, couponSvc, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.Handle("GET "+handler.RouteProducts, h.SearchProductsHandler())
	return mux
}

func newFullMux(store *memory.ProductStore) (*http.ServeMux, *handler.Handler) {
	svc := service.NewProductSearchService(store, nil)
	couponSvc := service.NewCouponService(memory.NewCouponStore())
	h := handler.NewHandler(svc, couponSvc, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.Handle("GET "+handler.RouteProducts, h.SearchProductsHandler())
	mux.Handle("POST "+handler.RouteProducts, h.CreateProductHandler())
	mux.Handle("PUT "+handler.RouteProductByID, h.SingleProductHandler())
	mux.Handle("DELETE "+handler.RouteProductByID, h.SingleProductHandler())
	mux.Handle("POST "+handler.RouteProductImages, h.UploadImageHandler())
	mux.Handle("POST "+handler.RouteVariants, h.CreateVariantHandler())
	mux.Handle("PUT "+handler.RouteVariantBySKU, h.VariantBySKUHandler())
	mux.Handle("DELETE "+handler.RouteVariantBySKU, h.VariantBySKUHandler())
	mux.Handle("POST "+handler.RouteVariantImages, h.UploadVariantImageHandler())
	return mux, h
}

func TestHealth(t *testing.T) {
	mux := newMux(memory.NewProductStore())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, handler.RouteHealth, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp handler.HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "ok" {
		t.Errorf("want ok, got %q", resp.Status)
	}
}

func TestSearchProductsParentsOnly(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BOT-001", Name: "Cassette", Brand: "Bottega", Category: "bags", Status: "active"},
	}
	store.Variants = []domain.Variant{
		{SKU: "BOT-001-GRN", ProductID: "BOT-001", Color: "Green", Price: 2500, Status: "active"},
		{SKU: "BOT-001-BLK", ProductID: "BOT-001", Color: "Black", Price: 2500, Status: "active"},
	}
	mux := newMux(store)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, handler.RouteProducts+"?category=bags", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp handler.SearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 {
		t.Fatalf("want total=1 (no color duplicates), got %d", resp.Total)
	}
	raw, _ := json.Marshal(resp.Results)
	var products []domain.Product
	json.Unmarshal(raw, &products)
	if len(products[0].AvailableColors) != 2 {
		t.Fatalf("want 2 available colors, got %v", products[0].AvailableColors)
	}
	if products[0].PriceFrom != 2500 {
		t.Fatalf("want priceFrom=2500, got %v", products[0].PriceFrom)
	}
}

func TestSearchProductsByColor(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BOT-001", Name: "Cassette", Category: "bags", Status: "active"},
		{ID: "GUC-001", Name: "Jackie", Category: "bags", Status: "active"},
	}
	store.Variants = []domain.Variant{
		{SKU: "BOT-001-GRN", ProductID: "BOT-001", Color: "Green", Status: "active"},
		{SKU: "GUC-001-BLK", ProductID: "GUC-001", Color: "Black", Status: "active"},
	}
	mux := newMux(store)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, handler.RouteProducts+"?color=Green", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp handler.SearchResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Total != 1 {
		t.Fatalf("want total=1, got %d", resp.Total)
	}
}

func TestSearchProductsByTags(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BAG-001", Name: "Hot Tote", Category: "bags", Status: "active", Tags: []string{"hot", "new"}},
		{ID: "BAG-002", Name: "Plain Tote", Category: "bags", Status: "active", Tags: []string{"new"}},
	}
	store.Variants = []domain.Variant{
		{SKU: "BAG-001", ProductID: "BAG-001", Status: "active"},
		{SKU: "BAG-002", ProductID: "BAG-002", Status: "active"},
	}
	mux := newMux(store)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, handler.RouteProducts+"?tags=hot", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp handler.SearchResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Total != 1 {
		t.Fatalf("want total=1, got %d", resp.Total)
	}
}

func TestCreateProductBrandPrefixID(t *testing.T) {
	mux, _ := newFullMux(memory.NewProductStore())

	body, _ := json.Marshal(domain.Product{
		Name:  "Mini Bag",
		Brand: "Bottega Veneta",
		Price: 2500,
		Color: "Green",
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, handler.RouteProducts, bytes.NewReader(body)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var p domain.Product
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if p.ID != "BOT-001" {
		t.Errorf("want BOT-001, got %q", p.ID)
	}
	if len(p.Variants) != 1 {
		t.Fatalf("want 1 default variant, got %d", len(p.Variants))
	}
	if p.Variants[0].SKU != "BOT-001" {
		t.Errorf("want default sku BOT-001, got %q", p.Variants[0].SKU)
	}
}

func TestCreateProductSequentialIDs(t *testing.T) {
	mux, _ := newFullMux(memory.NewProductStore())

	for i := 1; i <= 3; i++ {
		body, _ := json.Marshal(domain.Product{Name: fmt.Sprintf("Bag %d", i), Brand: "Gucci", Price: 100})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, handler.RouteProducts, bytes.NewReader(body)))
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %d: want 201, got %d", i, rec.Code)
		}
		var p domain.Product
		json.NewDecoder(rec.Body).Decode(&p)
		want := fmt.Sprintf("GUC-%03d", i)
		if p.ID != want {
			t.Errorf("create %d: want %s, got %q", i, want, p.ID)
		}
	}
}

func TestCreateVariant(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{{ID: "BOT-001", Name: "Cassette", Status: "active"}}
	store.Variants = []domain.Variant{{SKU: "BOT-001", ProductID: "BOT-001", Color: "Green", Status: "active"}}
	mux, _ := newFullMux(store)

	body, _ := json.Marshal(domain.Variant{Color: "Black", Price: 2500})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/products/BOT-001/variants", bytes.NewReader(body)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var v domain.Variant
	json.NewDecoder(rec.Body).Decode(&v)
	if v.Color != "Black" {
		t.Errorf("want Black, got %q", v.Color)
	}
	if v.SKU == "" {
		t.Fatal("want generated sku")
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/products/BOT-001", nil))
	var p domain.Product
	json.NewDecoder(rec.Body).Decode(&p)
	if len(p.Variants) != 2 {
		t.Fatalf("want 2 variants on PDP, got %d", len(p.Variants))
	}
	if len(p.AvailableColors) != 2 {
		t.Fatalf("want 2 colors, got %v", p.AvailableColors)
	}
}

func TestPublicGetProduct(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BOT-001", Name: "Mini Bag", Brand: "Bottega Veneta", Status: "active"},
	}
	store.Variants = []domain.Variant{
		{SKU: "BOT-001", ProductID: "BOT-001", Color: "Green", SellingPrice: 3000, Price: 2500, Status: "active"},
	}
	mux := newMux(store)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/products/BOT-001", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var p domain.Product
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if p.Price != 2500 {
		t.Fatalf("want price=2500, got %v", p.Price)
	}
	if p.SellingPrice != 3000 {
		t.Fatalf("want sellingPrice=3000, got %v", p.SellingPrice)
	}
	if len(p.Variants) != 1 {
		t.Fatalf("want variants on PDP, got %d", len(p.Variants))
	}
}

func TestPublicGetProductDraftHidden(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BOT-001", Name: "Mini Bag", Status: "draft"},
	}
	mux := newMux(store)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/products/BOT-001", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestDeleteProduct(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{{ID: "BOT-001", Name: "Mini Bag"}}
	store.Variants = []domain.Variant{{SKU: "BOT-001", ProductID: "BOT-001"}}
	mux, _ := newFullMux(store)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/products/BOT-001", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
	}
}

func TestUploadImageNoStore(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{{ID: "BOT-001", Name: "Mini Bag"}}
	store.Variants = []domain.Variant{{SKU: "BOT-001", ProductID: "BOT-001", Status: "active"}}
	mux, _ := newFullMux(store)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("image", "photo.jpg")
	fw.Write([]byte("fake-image-data"))
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/products/BOT-001/images", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "image store not configured") {
		t.Errorf("want 'image store not configured' in body, got: %s", rec.Body.String())
	}
}

func TestRedeemCoupon(t *testing.T) {
	mux := newMux(memory.NewProductStore())

	body, _ := json.Marshal(map[string]string{"code": "SUMMER30"})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, handler.RouteRedeemCoupon, bytes.NewReader(body)))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var coupon domain.Coupon
	if err := json.NewDecoder(rec.Body).Decode(&coupon); err != nil {
		t.Fatal(err)
	}
	if coupon.Code != "SUMMER30" {
		t.Errorf("want SUMMER30, got %q", coupon.Code)
	}
}
