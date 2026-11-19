package handler_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/elug3/dupli1/product/pkg/handler"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/middleware"
	"github.com/elug3/dupli1/product/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

func newReferralHTTPMux(t *testing.T) *http.ServeMux {
	t.Helper()
	store := memory.NewProductStore()
	promotionStore := memory.NewPromotionStore()
	promotionSvc := service.NewPromotionService(promotionStore).
		WithLedger(memory.NewPromotionRedemptionStore(promotionStore))
	validator := authjwt.NewHMACValidator(accessControlSecret)
	h := handler.NewHandler(
		service.NewProductSearchService(store, nil),
		promotionSvc,
		nil,
		service.NewCatalogService(store.Catalog),
	)
	requirePerm := func(perm string, next http.Handler) http.Handler {
		return middleware.RequireAuth(validator, middleware.RequireAnyPermission(perm)(next))
	}
	mux := http.NewServeMux()
	mux.Handle("POST "+handler.RoutePromotions, requirePerm(permissions.PromotionCreate, http.HandlerFunc(h.CreatePromotion)))
	mux.Handle("PUT "+handler.RoutePromotionByCode, requirePerm(permissions.PromotionUpdate, http.HandlerFunc(h.UpdatePromotion)))
	mux.Handle("POST "+handler.RouteRedeemPromotion, http.HandlerFunc(h.RedeemPromotion))
	mux.Handle("POST "+handler.RouteEvaluatePromotion, http.HandlerFunc(h.EvaluatePromotion))
	mux.Handle("POST "+handler.RouteReservePromotion, requirePerm(permissions.PromotionRedeem, http.HandlerFunc(h.ReservePromotion)))
	return mux
}

var referralCreateBody = map[string]any{
	"code":          "HUH15",
	"kind":          "referral",
	"referrer_name": "허경호",
	"benefit": map[string]any{
		"target":            "goods",
		"discount_type":     "percent",
		"discount_fraction": 0.15,
		"apply_to":          "entire_subtotal",
	},
}

var referralCart = map[string]any{
	"code":             "HUH15",
	"customer_id":      "cust-1",
	"shipping_fee_won": 30000,
	"lines": []map[string]any{
		{"sku_id": "sku-1", "quantity": 1, "unit_price_won": 1000000},
	},
}

func TestCreateReferralCodeDefaultsTheCommissionToFivePercent(t *testing.T) {
	mux := newReferralHTTPMux(t)
	manager := makeAccessToken(t, "mgr-1", []string{permissions.PromotionCreate, permissions.PromotionUpdate})
	w := serve(t, mux, http.MethodPost, handler.RoutePromotions, manager, referralCreateBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		Kind          string `json:"kind"`
		ReferrerName  string `json:"referrer_name"`
		CommissionBps int    `json:"commission_bps"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.Kind != "referral" || created.ReferrerName != "허경호" || created.CommissionBps != 500 {
		t.Fatalf("created = %+v, want referral by 허경호 at 500 bps", created)
	}

	// The kind is fixed once created.
	w = serve(t, mux, http.MethodPut, "/api/v1/products/promotions/by-code/HUH15", manager, map[string]any{"kind": ""})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("changing kind status = %d, want 400", w.Code)
	}
}

func TestReferralCodeLooksLikeAnyCodeToShoppers(t *testing.T) {
	mux := newReferralHTTPMux(t)
	manager := makeAccessToken(t, "mgr-1", []string{permissions.PromotionCreate})
	if w := serve(t, mux, http.MethodPost, handler.RoutePromotions, manager, referralCreateBody); w.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", w.Code, w.Body.String())
	}

	for _, route := range []string{handler.RouteRedeemPromotion, handler.RouteEvaluatePromotion} {
		w := serve(t, mux, http.MethodPost, route, "", referralCart)
		if w.Code != http.StatusOK {
			t.Fatalf("%s status = %d: %s", route, w.Code, w.Body.String())
		}
		body := w.Body.String()
		for _, secret := range []string{"referral", "referrer", "commission", "허경호"} {
			if strings.Contains(body, secret) {
				t.Fatalf("%s leaks %q: %s", route, secret, body)
			}
		}
	}

	// Order's internal reserve does carry it, for the commission report.
	orderSvc := makeAccessToken(t, "order-svc", []string{permissions.PromotionRedeem})
	reserve := map[string]any{}
	for k, v := range referralCart {
		reserve[k] = v
	}
	reserve["order_id"] = "ord-1"
	w := serve(t, mux, http.MethodPost, handler.RouteReservePromotion, orderSvc, reserve)
	if w.Code != http.StatusOK {
		t.Fatalf("reserve status = %d: %s", w.Code, w.Body.String())
	}
	var got struct {
		Result struct {
			OK          bool  `json:"ok"`
			DiscountWon int64 `json:"discount_won"`
		} `json:"result"`
		Referral *struct {
			ReferrerName  string `json:"referrer_name"`
			CommissionBps int    `json:"commission_bps"`
		} `json:"referral"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if !got.Result.OK || got.Result.DiscountWon != 150000 || got.Referral == nil ||
		got.Referral.ReferrerName != "허경호" || got.Referral.CommissionBps != 500 {
		t.Fatalf("reserve = %s", w.Body.String())
	}
}
