package handler_test

import (
	"net/http"
	"testing"

	"github.com/elug3/dupli1/product/pkg/handler"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
)

func TestRouteProductsAllIsNotPublicPDP(t *testing.T) {
	mux := newAccessControlMux(memory.NewProductStore())

	w := serve(t, mux, http.MethodGet, handler.RouteProductsAll, "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/v1/products/all without auth: status=%d, want 401", w.Code)
	}

	w = serve(t, mux, http.MethodGet, handler.RouteProducts, "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/v1/products without auth: status=%d, want 401", w.Code)
	}
}
