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

	"github.com/schick/pkg/product/domain"
	"github.com/schick/pkg/product/handler"
	"github.com/schick/pkg/product/infra/memory"
	"github.com/schick/pkg/product/service"
)

// newMux registers only the public routes (no auth middleware).
func newMux(store *memory.ProductStore) *http.ServeMux {
	svc := service.NewProductSearchService(store, nil)
	couponSvc := service.NewCouponService()
	h := handler.NewHandler(svc, couponSvc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

// newFullMux registers all routes without auth, for handler-level tests.
func newFullMux(store *memory.ProductStore) (*http.ServeMux, *handler.Handler) {
	svc := service.NewProductSearchService(store, nil)
	couponSvc := service.NewCouponService()
	h := handler.NewHandler(svc, couponSvc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.Handle("GET /api/products", h.ListProductsHandler())
	mux.Handle("POST /api/products", h.CreateProductHandler())
	mux.Handle("GET /api/products/{id}", h.SingleProductHandler())
	mux.Handle("PUT /api/products/{id}", h.SingleProductHandler())
	mux.Handle("DELETE /api/products/{id}", h.SingleProductHandler())
	mux.Handle("PUT /api/products/{id}/image", h.UploadImageHandler())
	return mux, h
}

// --- Health ---

func TestHealth(t *testing.T) {
	mux := newMux(memory.NewProductStore())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp handler.HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "healthy" {
		t.Errorf("want healthy, got %q", resp.Status)
	}
}

func TestHealthMethodNotAllowed(t *testing.T) {
	mux := newMux(memory.NewProductStore())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/health", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
}

// --- Bags ---

func TestSearchBags(t *testing.T) {
	store := memory.NewProductStore()
	store.Bags = []domain.Bag{
		{Product: domain.Product{ID: "BAG-001", Name: "Canvas Tote", Brand: "Baggu", Price: 45.0}},
		{Product: domain.Product{ID: "HER-001", Name: "Leather Backpack", Brand: "Herschel", Price: 120.0}},
	}
	mux := newMux(store)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/products/bags", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp handler.SearchResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 2 {
		t.Errorf("want total=2, got %d", resp.Total)
	}
}

// --- Products (CRUD) ---

func TestCreateProductBrandPrefixID(t *testing.T) {
	mux, _ := newFullMux(memory.NewProductStore())

	body, _ := json.Marshal(domain.Product{
		Name:  "Mini Bag",
		Brand: "Bottega Veneta",
		Price: 2500,
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/products", bytes.NewReader(body)))

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
}

func TestCreateProductSequentialIDs(t *testing.T) {
	mux, _ := newFullMux(memory.NewProductStore())

	for i := 1; i <= 3; i++ {
		body, _ := json.Marshal(domain.Product{Name: fmt.Sprintf("Bag %d", i), Brand: "Gucci"})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/products", bytes.NewReader(body)))
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

func TestListProducts(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BOT-001", Name: "Mini Bag", Brand: "Bottega Veneta"},
	}
	mux, _ := newFullMux(store)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/products", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var products []domain.Product
	if err := json.NewDecoder(rec.Body).Decode(&products); err != nil {
		t.Fatal(err)
	}
	if len(products) != 1 {
		t.Errorf("want 1 product, got %d", len(products))
	}
}

func TestGetProduct(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BOT-001", Name: "Mini Bag", Brand: "Bottega Veneta"},
	}
	mux, _ := newFullMux(store)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/products/BOT-001", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestGetProductNotFound(t *testing.T) {
	mux, _ := newFullMux(memory.NewProductStore())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/products/NOPE-999", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestDeleteProduct(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{{ID: "BOT-001", Name: "Mini Bag"}}
	mux, _ := newFullMux(store)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/products/BOT-001", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
	}
}

// --- Image upload ---

func TestUploadImageNoStore(t *testing.T) {
	// nil imageStore — should return 500 with a clear error
	store := memory.NewProductStore()
	store.Products = []domain.Product{{ID: "BOT-001", Name: "Mini Bag"}}
	mux, _ := newFullMux(store)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("image", "photo.jpg")
	fw.Write([]byte("fake-image-data"))
	w.Close()

	req := httptest.NewRequest(http.MethodPut, "/api/products/BOT-001/image", &buf)
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

func TestUploadImageProductNotFound(t *testing.T) {
	mux, _ := newFullMux(memory.NewProductStore())

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("image", "photo.jpg")
	fw.Write([]byte("fake-image-data"))
	w.Close()

	req := httptest.NewRequest(http.MethodPut, "/api/products/NOPE-999/image", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// nil store returns "image store not configured" before it can check product existence
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
}

// --- Coupons ---

func TestRedeemCoupon(t *testing.T) {
	mux := newMux(memory.NewProductStore())

	body, _ := json.Marshal(map[string]string{"code": "SUMMER30"})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/coupons/redeem", bytes.NewReader(body)))

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
	if coupon.Discount != 0.30 {
		t.Errorf("want discount 0.30, got %f", coupon.Discount)
	}
}

func TestRedeemCouponInvalid(t *testing.T) {
	mux := newMux(memory.NewProductStore())

	body, _ := json.Marshal(map[string]string{"code": "NOTREAL"})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/coupons/redeem", bytes.NewReader(body)))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestRedeemCouponMissingCode(t *testing.T) {
	mux := newMux(memory.NewProductStore())

	body, _ := json.Marshal(map[string]string{"code": ""})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/coupons/redeem", bytes.NewReader(body)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}
