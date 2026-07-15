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
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/golang-jwt/jwt/v5"
)

const accessControlSecret = "product-access-control-secret"

func makeAccessToken(t *testing.T, userID string, perms []string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":         userID,
		"permissions": perms,
		"type":        "access",
		"exp":         time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(accessControlSecret))
	if err != nil {
		t.Fatalf("makeAccessToken: %v", err)
	}
	return signed
}

func bearer(token string) string { return "Bearer " + token }

func newAccessControlMux(store *memory.ProductStore) *http.ServeMux {
	validator := authjwt.NewHMACValidator(accessControlSecret)
	svc := service.NewProductSearchService(store, nil)
	couponSvc := service.NewCouponService(memory.NewCouponStore())
	h := handler.NewHandler(svc, couponSvc, nil, service.NewCatalogService(memory.NewCatalogStore()))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	requirePerm := func(perm string, next http.Handler) http.Handler {
		return middleware.RequireAuth(validator, middleware.RequireAnyPermission(perm)(next))
	}

	mux.Handle("GET "+handler.RouteProducts, middleware.OptionalAuth(validator, h.SearchProductsHandler()))
	mux.Handle("POST "+handler.RouteProducts, requirePerm(permissions.ProductCreate, h.CreateProductHandler()))
	mux.Handle("PUT "+handler.RouteProductByID, requirePerm(permissions.ProductUpdate, h.SingleProductHandler()))
	mux.Handle("DELETE "+handler.RouteProductByID, requirePerm(permissions.ProductDelete, h.SingleProductHandler()))
	mux.Handle("POST "+handler.RouteProductImages, requirePerm(permissions.ProductImageUpload, h.UploadImageHandler()))
	mux.Handle("GET "+handler.RouteCoupons, requirePerm(permissions.CouponRead, http.HandlerFunc(h.ListCoupons)))
	mux.Handle("POST "+handler.RouteCoupons, requirePerm(permissions.CouponCreate, http.HandlerFunc(h.CreateCoupon)))
	mux.Handle("PUT "+handler.RouteCouponByCode, requirePerm(permissions.CouponUpdate, http.HandlerFunc(h.UpdateCoupon)))
	mux.Handle("DELETE "+handler.RouteCouponByCode, requirePerm(permissions.CouponDelete, http.HandlerFunc(h.DeleteCoupon)))

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
		{http.MethodGet, handler.RouteProducts},
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
		{http.MethodPost, handler.RouteProducts},
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

	req := httptest.NewRequest(http.MethodPost, handler.RouteProducts, nil)
	req.Header.Set("Authorization", "Bearer not.a.valid.jwt")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestCustomerCannotManageProducts(t *testing.T) {
	mux := newAccessControlMux(memory.NewProductStore())
	token := makeAccessToken(t, "cust-1", nil)

	body := domain.Product{Name: "Mini Bag", Brand: "Gucci", Price: 100}
	w := serve(t, mux, http.MethodPost, handler.RouteProducts, token, body)
	if w.Code != http.StatusForbidden {
		t.Fatalf("create product: status = %d, want 403", w.Code)
	}

	w = serve(t, mux, http.MethodGet, handler.RouteProducts, token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list products: status = %d, want 200 (public search)", w.Code)
	}

	w = serve(t, mux, http.MethodGet, handler.RouteCoupons, token, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("list coupons: status = %d, want 403", w.Code)
	}
}

func TestProductManagerCanManageProducts(t *testing.T) {
	mux := newAccessControlMux(memory.NewProductStore())
	token := makeAccessToken(t, "mgr-1", permissions.ExpandLegacyRoles([]string{permissions.RoleProductManager}))

	body := domain.Product{Name: "Mini Bag", Brand: "Gucci", Price: 100}
	w := serve(t, mux, http.MethodPost, handler.RouteProducts, token, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create product: status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	var created domain.Product
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode created product: %v", err)
	}
	if created.CreatedBy != "mgr-1" {
		t.Fatalf("createdBy = %q, want mgr-1", created.CreatedBy)
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
	token := makeAccessToken(t, "admin-1", permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin}))

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
	token := makeAccessToken(t, "owner-1", []string{permissions.All})

	body := domain.Product{Name: "Backpack", Brand: "Herschel", Price: 120}
	w := serve(t, mux, http.MethodPost, handler.RouteProducts, token, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
}
