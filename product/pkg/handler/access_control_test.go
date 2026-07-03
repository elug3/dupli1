package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elug3/dupli1/product/pkg/authjwt"
	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/handler"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/middleware"
	"github.com/elug3/dupli1/product/pkg/service"
	"github.com/golang-jwt/jwt/v5"
)

const accessControlSecret = "product-access-control-secret"

func makeAccessToken(t *testing.T, userID string, roles []string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":   userID,
		"roles": roles,
		"type":  "access",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(accessControlSecret))
	if err != nil {
		t.Fatalf("makeAccessToken: %v", err)
	}
	return signed
}

func bearer(token string) string { return "Bearer " + token }

// newAccessControlMux registers public and protected routes with auth middleware,
// matching product/pkg/bootstrap/bootstrap.go wiring.
func newAccessControlMux(store *memory.ProductStore) *http.ServeMux {
	validator := authjwt.NewHMACValidator(accessControlSecret)
	svc := service.NewProductSearchService(store, nil)
	couponSvc := service.NewCouponService(memory.NewCouponStore())
	h := handler.NewHandler(svc, couponSvc)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	protect := func(next http.Handler) http.Handler {
		return middleware.RequireAuth(validator,
			middleware.RequireAnyRole(middleware.ProductManagerRoles...)(next))
	}

	mux.Handle("GET "+handler.RouteProducts, protect(h.ListProductsHandler()))
	mux.Handle("POST "+handler.RouteProducts, protect(h.CreateProductHandler()))
	mux.Handle("GET "+handler.RouteManageProduct, protect(h.GetProductHandler()))
	mux.Handle("PUT "+handler.RouteProductByID, protect(h.SingleProductHandler()))
	mux.Handle("DELETE "+handler.RouteProductByID, protect(h.SingleProductHandler()))
	mux.Handle("PUT "+handler.RouteProductImage, protect(h.UploadImageHandler()))
	mux.Handle("GET "+handler.RouteCoupons, protect(http.HandlerFunc(h.ListCoupons)))
	mux.Handle("POST "+handler.RouteCoupons, protect(http.HandlerFunc(h.CreateCoupon)))
	mux.Handle("PUT "+handler.RouteCouponByCode, protect(http.HandlerFunc(h.UpdateCoupon)))
	mux.Handle("DELETE "+handler.RouteCouponByCode, protect(http.HandlerFunc(h.DeleteCoupon)))

	return mux
}

func serve(t *testing.T, mux *http.ServeMux, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if token != "" {
		req.Header.Set("Authorization", bearer(token))
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestPublicRoutesDoNotRequireAuth(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{
		{ID: "BOT-001", Name: "Mini Bag", Brand: "Bottega Veneta", Status: "active"},
	}
	mux := newAccessControlMux(store)

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, handler.RouteHealth},
		{http.MethodGet, handler.RouteSearchBags},
		{http.MethodGet, "/api/v1/products/BOT-001"},
		{http.MethodPost, handler.RouteRedeemCoupon},
	}

	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			var body any
			if tc.path == handler.RouteRedeemCoupon {
				body = map[string]string{"code": "SUMMER30"}
			}
			w := serve(t, mux, tc.method, tc.path, "", body)
			if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
				t.Fatalf("status = %d, want a public success response", w.Code)
			}
		})
	}
}

func TestProtectedRoutesRejectMissingToken(t *testing.T) {
	mux := newAccessControlMux(memory.NewProductStore())

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, handler.RouteProducts},
		{http.MethodPost, handler.RouteProducts},
		{http.MethodGet, "/api/v1/products/BOT-001/manage"},
		{http.MethodGet, handler.RouteCoupons},
	}

	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			w := serve(t, mux, tc.method, tc.path, "", nil)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", w.Code)
			}
		})
	}
}

func TestProtectedRoutesRejectInvalidToken(t *testing.T) {
	mux := newAccessControlMux(memory.NewProductStore())

	req := httptest.NewRequest(http.MethodGet, handler.RouteProducts, nil)
	req.Header.Set("Authorization", "Bearer not.a.valid.jwt")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestCustomerCannotManageProducts(t *testing.T) {
	mux := newAccessControlMux(memory.NewProductStore())
	token := makeAccessToken(t, "cust-1", []string{"customer"})

	body := domain.Product{Name: "Mini Bag", Brand: "Gucci", Price: 100}
	w := serve(t, mux, http.MethodPost, handler.RouteProducts, token, body)
	if w.Code != http.StatusForbidden {
		t.Fatalf("create product: status = %d, want 403", w.Code)
	}

	w = serve(t, mux, http.MethodGet, handler.RouteProducts, token, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("list products: status = %d, want 403", w.Code)
	}

	w = serve(t, mux, http.MethodGet, handler.RouteCoupons, token, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("list coupons: status = %d, want 403", w.Code)
	}
}

func TestProductManagerCanManageProducts(t *testing.T) {
	mux := newAccessControlMux(memory.NewProductStore())
	token := makeAccessToken(t, "mgr-1", []string{"product_manager"})

	body := domain.Product{Name: "Mini Bag", Brand: "Gucci", Price: 100}
	w := serve(t, mux, http.MethodPost, handler.RouteProducts, token, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create product: status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	w = serve(t, mux, http.MethodGet, handler.RouteProducts, token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list products: status = %d, want 200", w.Code)
	}

	w = serve(t, mux, http.MethodGet, handler.RouteCoupons, token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list coupons: status = %d, want 200", w.Code)
	}
}

func TestAdminCanManageProducts(t *testing.T) {
	mux := newAccessControlMux(memory.NewProductStore())
	token := makeAccessToken(t, "admin-1", []string{"admin"})

	body := domain.Product{Name: "Tote", Brand: "Baggu", Price: 45}
	w := serve(t, mux, http.MethodPost, handler.RouteProducts, token, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	w = serve(t, mux, http.MethodGet, handler.RouteProducts, token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list products: status = %d, want 200", w.Code)
	}
}

func TestOwnerCanManageProducts(t *testing.T) {
	mux := newAccessControlMux(memory.NewProductStore())
	token := makeAccessToken(t, "owner-1", []string{"owner", "product_manager"})

	body := domain.Product{Name: "Backpack", Brand: "Herschel", Price: 120}
	w := serve(t, mux, http.MethodPost, handler.RouteProducts, token, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
}
