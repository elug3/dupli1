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
	if store.Catalog == nil {
		store.Catalog = memory.NewCatalogStore()
	}
	svc := service.NewProductSearchService(store, nil)
	couponSvc := service.NewCouponService(memory.NewCouponStore())
	catalogSvc := service.NewCatalogService(store.Catalog)
	h := handler.NewHandler(svc, couponSvc, nil, catalogSvc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.Handle("GET "+handler.RouteProducts, h.SearchProductsHandler())
	return mux
}

func newFullMux(store *memory.ProductStore) (*http.ServeMux, *handler.Handler) {
	if store.Catalog == nil {
		store.Catalog = memory.NewCatalogStore()
	}
	svc := service.NewProductSearchService(store, nil)
	couponSvc := service.NewCouponService(memory.NewCouponStore())
	catalogSvc := service.NewCatalogService(store.Catalog)
	h := handler.NewHandler(svc, couponSvc, nil, catalogSvc)
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
	mux.Handle("GET "+handler.RouteCatalogBrands, http.HandlerFunc(h.ListBrands))
	mux.Handle("POST "+handler.RouteCatalogBrands, http.HandlerFunc(h.CreateBrand))
	mux.Handle("PATCH "+handler.RouteCatalogBrandByCode, http.HandlerFunc(h.UpdateBrand))
	mux.Handle("DELETE "+handler.RouteCatalogBrandByCode, http.HandlerFunc(h.DeleteBrand))
	mux.Handle("GET "+handler.RouteCatalogStyles, http.HandlerFunc(h.ListStyles))
	mux.Handle("POST "+handler.RouteCatalogStyles, http.HandlerFunc(h.CreateStyle))
	mux.Handle("PATCH "+handler.RouteCatalogStyleByCode, http.HandlerFunc(h.UpdateStyle))
	mux.Handle("DELETE "+handler.RouteCatalogStyleByCode, http.HandlerFunc(h.DeleteStyle))
	mux.Handle("GET "+handler.RouteCatalogColors, http.HandlerFunc(h.ListColors))
	mux.Handle("POST "+handler.RouteCatalogColors, http.HandlerFunc(h.CreateColor))
	mux.Handle("PATCH "+handler.RouteCatalogColorByCode, http.HandlerFunc(h.UpdateColor))
	mux.Handle("DELETE "+handler.RouteCatalogColorByCode, http.HandlerFunc(h.DeleteColor))
	return mux, h
}

func seedCatalogStyle(t *testing.T, catalog *memory.CatalogStore, brandCode, styleCode, name string) {
	t.Helper()
	if _, err := catalog.CreateStyle(domain.Style{BrandCode: brandCode, Code: styleCode, Name: name}); err != nil {
		t.Fatalf("seed style %s/%s: %v", brandCode, styleCode, err)
	}
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

func TestSettings(t *testing.T) {
	mux := newMux(memory.NewProductStore())
	for _, path := range []string{handler.RouteSettings, handler.RouteInventorySettings} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s want 200, got %d", path, rec.Code)
		}
		var body map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("%s decode: %v", path, err)
		}
		if body["service"] != "product" {
			t.Fatalf("%s service = %v, want product", path, body["service"])
		}
		if body["api_version"] != "v1" {
			t.Fatalf("%s api_version = %v, want v1", path, body["api_version"])
		}
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

func TestCreateProductUsesULID(t *testing.T) {
	store := memory.NewProductStore()
	seedCatalogStyle(t, store.Catalog, "BOT", "MINI01", "Mini Bag")
	mux, _ := newFullMux(store)

	body, _ := json.Marshal(domain.Product{
		Name: "Mini Bag", Brand: "Bottega Veneta", StyleCode: "MINI01",
		Price: 2500, Color: "Green",
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
	if len(p.ID) != 26 {
		t.Errorf("want ULID product id (26 chars), got %q", p.ID)
	}
	if p.BrandCode != "BOT" || p.StyleCode != "MINI01" {
		t.Errorf("codes: brand=%q style=%q", p.BrandCode, p.StyleCode)
	}
	if len(p.Variants) != 1 {
		t.Fatalf("want 1 default variant, got %d", len(p.Variants))
	}
	if p.Variants[0].SKU != "BOT_MINI01_GRN_OS" {
		t.Errorf("want composed sku BOT_MINI01_GRN_OS, got %q", p.Variants[0].SKU)
	}
}

func TestCreateProductUniqueULIDs(t *testing.T) {
	store := memory.NewProductStore()
	mux, _ := newFullMux(store)
	ids := map[string]bool{}
	for i := 1; i <= 3; i++ {
		style := fmt.Sprintf("S%03d", i)
		seedCatalogStyle(t, store.Catalog, "GUC", style, fmt.Sprintf("Bag %d", i))
		body, _ := json.Marshal(domain.Product{
			Name: fmt.Sprintf("Bag %d", i), Brand: "Gucci", StyleCode: style, Price: 100, Color: "Black",
		})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, handler.RouteProducts, bytes.NewReader(body)))
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %d: want 201, got %d: %s", i, rec.Code, rec.Body.String())
		}
		var p domain.Product
		json.NewDecoder(rec.Body).Decode(&p)
		if ids[p.ID] {
			t.Fatalf("duplicate product id %q", p.ID)
		}
		ids[p.ID] = true
		if len(p.ID) != 26 {
			t.Errorf("create %d: want ULID, got %q", i, p.ID)
		}
	}
}

func TestCreateVariant(t *testing.T) {
	store := memory.NewProductStore()
	seedCatalogStyle(t, store.Catalog, "BOT", "CAS001", "Cassette")
	store.Products = []domain.Product{{
		ID: "BOT-001", Name: "Cassette", BrandCode: "BOT", StyleCode: "CAS001", Status: "active",
	}}
	store.Variants = []domain.Variant{{
		SKU: "BOT_CAS001_GRN_OS", ProductID: "BOT-001", Color: "Green", ColorCode: "GRN", SizeCode: "OS", Status: "active",
	}}
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
	if v.SKU != "BOT_CAS001_BLK_OS" {
		t.Fatalf("want BOT_CAS001_BLK_OS, got %q", v.SKU)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/products/BOT-001", nil))
	var p domain.Product
	json.NewDecoder(rec.Body).Decode(&p)
	if len(p.Variants) != 2 {
		t.Fatalf("want 2 variants on PDP, got %d", len(p.Variants))
	}
	if len(p.AvailableColors) != 2 {
		t.Fatalf("want 2 availableColors, got %d", len(p.AvailableColors))
	}
}

func TestCreateVariantLuxurySKU(t *testing.T) {
	store := memory.NewProductStore()
	seedCatalogStyle(t, store.Catalog, "BOT", "CAS001", "Cassette")
	store.Products = []domain.Product{{
		ID: "BOT-001", Name: "Cassette", Brand: "Bottega Veneta",
		BrandCode: "BOT", StyleCode: "CAS001", Status: "active",
	}}
	mux, _ := newFullMux(store)

	body, _ := json.Marshal(domain.Variant{
		Color: "Black", Size: "Medium", EditionCode: "V", Price: 2500,
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/products/BOT-001/variants", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var v domain.Variant
	json.NewDecoder(rec.Body).Decode(&v)
	if v.SKU != "BOT_CAS001_BLK_V_MED" {
		t.Fatalf("want BOT_CAS001_BLK_V_MED, got %q (colorCode=%q sizeCode=%q edition=%q)",
			v.SKU, v.ColorCode, v.SizeCode, v.EditionCode)
	}
	if v.ColorCode != "BLK" || v.SizeCode != "MED" || v.EditionCode != "V" {
		t.Fatalf("unexpected codes: %+v", v)
	}
}

func TestCreateProductRequiresStyle(t *testing.T) {
	mux, _ := newFullMux(memory.NewProductStore())
	body, _ := json.Marshal(domain.Product{
		Name: "Mini Bag", Brand: "Bottega Veneta", Price: 2500, Color: "Green",
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, handler.RouteProducts, bytes.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 without styleCode, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateProductAssignsBrandAndStyleCodes(t *testing.T) {
	store := memory.NewProductStore()
	seedCatalogStyle(t, store.Catalog, "BOT", "S001", "Mini Bag")
	mux, _ := newFullMux(store)

	body, _ := json.Marshal(domain.Product{
		Name: "Mini Bag", Brand: "Bottega Veneta", StyleCode: "S001", Price: 2500, Color: "Green",
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, handler.RouteProducts, bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var p domain.Product
	json.NewDecoder(rec.Body).Decode(&p)
	if p.BrandCode != "BOT" {
		t.Errorf("brandCode: want BOT, got %q", p.BrandCode)
	}
	if p.StyleCode != "S001" {
		t.Errorf("styleCode: want S001, got %q", p.StyleCode)
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
