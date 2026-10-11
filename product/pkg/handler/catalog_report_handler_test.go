package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/handler"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/middleware"
	"github.com/elug3/dupli1/product/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

func TestCatalogReport_RequiresProductReadAndValidatesStatus(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{{ID: "p1", Brand: "Prada", Category: "bags", SubCategory: "tote", Status: "active"}}
	h := handler.NewHandler(service.NewProductSearchService(store, nil), service.NewPromotionService(memory.NewPromotionStore()), nil, service.NewCatalogService(store.Catalog)).
		WithCatalogReportService(service.NewCatalogReportService(store, store.Catalog))
	validator := authjwt.NewHMACValidator(accessControlSecret)
	mux := http.NewServeMux()
	// Mirrors bootstrap.
	mux.Handle("GET "+handler.RouteCatalogReport, middleware.RequireAuth(validator, middleware.RequireAnyPermission(permissions.ProductRead)(http.HandlerFunc(h.CatalogReport))))

	if w := serve(t, mux, http.MethodGet, handler.RouteCatalogReport, "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: status=%d, want 401", w.Code)
	}
	if w := serve(t, mux, http.MethodGet, handler.RouteCatalogReport, makeAccessToken(t, "cust", nil), nil); w.Code != http.StatusForbidden {
		t.Fatalf("customer: status=%d, want 403", w.Code)
	}
	mgr := makeAccessToken(t, "mgr", []string{permissions.ProductRead})
	if w := serve(t, mux, http.MethodGet, handler.RouteCatalogReport+"?status=gone", mgr, nil); w.Code != http.StatusBadRequest {
		t.Fatalf("bad status: status=%d, want 400", w.Code)
	}
	w := serve(t, mux, http.MethodGet, handler.RouteCatalogReport+"?status=active", mgr, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET report: status=%d body=%s", w.Code, w.Body.String())
	}
	var report domain.CatalogReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if report.Totals.Products != 1 || len(report.Cells) != 1 || report.Cells[0].SubCategory != "tote" {
		t.Fatalf("report=%+v", report)
	}
}
