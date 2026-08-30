package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/handler"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/service"
)

func newInventoryMux(t *testing.T) (*http.ServeMux, *service.InventoryService) {
	t.Helper()
	products := memory.NewProductStore()
	products.Products = []domain.Product{{ID: "BOT-001", Name: "Cassette", Status: "active"}}
	products.Variants = []domain.Variant{
		{SkuID: "SKUID-GRN", SKU: "BOT-001-GRN", ProductID: "BOT-001", Color: "Green", Price: 2500, Status: "active"},
	}
	invStore := memory.NewInventoryStore().WithProducts(products)
	invSvc := service.NewInventoryService(invStore, products)

	svc := service.NewProductSearchService(products, nil)
	h := handler.NewHandler(svc, service.NewCouponService(memory.NewCouponStore()), invSvc, service.NewCatalogService(memory.NewCatalogStore()))

	mux := http.NewServeMux()
	mux.Handle("POST "+handler.RouteInventoryReservationCommit, h.CommitReservationHandler())
	mux.Handle("POST "+handler.RouteInventoryReservationRelease, h.ReleaseReservationHandler())
	mux.Handle("POST "+handler.RouteInventoryReservations, h.CreateReservationHandler())
	mux.Handle("PUT "+handler.RouteInventoryItem, h.UpsertInventoryItemHandler())
	return mux, invSvc
}

func TestCommitReservationHandler_RejectsAlreadyReleased(t *testing.T) {
	mux, invSvc := newInventoryMux(t)
	ctx := t.Context()

	if _, err := invSvc.UpsertItem(ctx, service.SkuRef{SkuID: "SKUID-GRN"}, 5); err != nil {
		t.Fatalf("UpsertItem: %v", err)
	}
	reservation, err := invSvc.Reserve(ctx, "order-ship-race", []service.ReservationItemRef{
		{Ref: service.SkuRef{SkuID: "SKUID-GRN"}, Quantity: 1},
	})
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if _, err := invSvc.ReleaseReservation(ctx, reservation.ID); err != nil {
		t.Fatalf("ReleaseReservation: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.test/api/v1/products/inventory/reservations/"+reservation.ID+"/commit", nil)
	req.SetPathValue("id", reservation.ID)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !strings.Contains(body.Error, "already released") {
		t.Fatalf("error = %q, want reservation already released", body.Error)
	}
}
