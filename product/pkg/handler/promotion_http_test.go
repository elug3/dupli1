package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/handler"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/middleware"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/elug3/dupli1/product/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

func newPromotionHTTPMux(t *testing.T) (*http.ServeMux, *service.PromotionService) {
	t.Helper()
	store := memory.NewProductStore()
	promotionStore := memory.NewPromotionStore()
	promotionSvc := service.NewPromotionService(promotionStore).
		WithLedger(memory.NewPromotionRedemptionStore(promotionStore)).
		WithEntitlements(memory.NewPromotionEntitlementStore())
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
	mux.Handle("POST "+handler.RouteEvaluatePromotion, http.HandlerFunc(h.EvaluatePromotion))
	mux.Handle("POST "+handler.RouteReservePromotion, requirePerm(permissions.PromotionRedeem, http.HandlerFunc(h.ReservePromotion)))
	mux.Handle("POST "+handler.RouteConsumePromotion, requirePerm(permissions.PromotionRedeem, http.HandlerFunc(h.ConsumePromotion)))
	mux.Handle("POST "+handler.RouteReleasePromotion, requirePerm(permissions.PromotionRedeem, http.HandlerFunc(h.ReleasePromotion)))
	mux.Handle("POST "+handler.RoutePromotionWallet, middleware.RequireAuth(validator, http.HandlerFunc(h.PromotionWallet)))
	mux.Handle("GET "+handler.RoutePromotionWallet, middleware.RequireAuth(validator, http.HandlerFunc(h.PromotionWallet)))
	mux.Handle("POST "+handler.RoutePromotionIssue, requirePerm(permissions.PromotionIssue, http.HandlerFunc(h.IssuePromotion)))
	mux.Handle("DELETE "+handler.RoutePromotionEntitlement, requirePerm(permissions.PromotionIssue, http.HandlerFunc(h.RevokePromotionEntitlement)))
	return mux, promotionSvc
}

func fixedGlobalPromotion(code string, amount int64) domain.Promotion {
	return domain.Promotion{
		Code:   code,
		Scope:  domain.ScopeGlobal,
		Active: true,
		Benefit: domain.Benefit{
			Target:           domain.BenefitTargetGoods,
			DiscountType:     domain.DiscountTypeFixed,
			DiscountFixedWon: amount,
			ApplyTo:          domain.ApplyToEntireSubtotal,
		},
	}
}

func TestEvaluatePromotionIsPublicAndReturnsDiscount(t *testing.T) {
	mux, svc := newPromotionHTTPMux(t)
	if _, err := svc.Create(t.Context(), fixedGlobalPromotion("SAVE5K", 5000)); err != nil {
		t.Fatalf("create: %v", err)
	}

	body := map[string]any{
		"code":             "SAVE5K",
		"customer_id":      "cust-1",
		"shipping_fee_won": 3000,
		"lines": []map[string]any{
			{"sku_id": "sku-1", "sku": "sku-1", "quantity": 1, "unit_price_won": 50000},
		},
	}
	w := serve(t, mux, http.MethodPost, handler.RouteEvaluatePromotion, "", body)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var result domain.EvaluationResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !result.OK || result.DiscountWon != 5000 {
		t.Fatalf("result = %+v, want ok with 5000 discount", result)
	}
}

func TestEvaluatePromotionRequiresCode(t *testing.T) {
	mux, _ := newPromotionHTTPMux(t)
	w := serve(t, mux, http.MethodPost, handler.RouteEvaluatePromotion, "", map[string]string{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestPromotionWalletRequiresAuth(t *testing.T) {
	mux, _ := newPromotionHTTPMux(t)
	w := serve(t, mux, http.MethodGet, handler.RoutePromotionWallet, "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestPromotionWalletListsEntitlementsForTokenSubject(t *testing.T) {
	mux, svc := newPromotionHTTPMux(t)
	active := true
	if _, err := svc.Update(t.Context(), "WELCOME50", ports.PromotionPatch{Active: &active}); err != nil {
		t.Fatalf("enable WELCOME50: %v", err)
	}
	if _, err := svc.Issue(t.Context(), "WELCOME50", "cust-wallet", "system", "k1", ""); err != nil {
		t.Fatalf("Issue: %v", err)
	}

	token := makeAccessToken(t, "cust-wallet", nil)
	body := map[string]any{
		"shipping_fee_won": 3000,
		"lines": []map[string]any{
			{"sku_id": "sku-1", "quantity": 1, "unit_price_won": 150000},
		},
	}
	w := serve(t, mux, http.MethodPost, handler.RoutePromotionWallet, token, body)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Total   int `json:"total"`
		Results []struct {
			Eligible    bool  `json:"eligible"`
			DiscountWon int64 `json:"discount_won"`
			Entitlement struct {
				Code string `json:"code"`
			} `json:"entitlement"`
		} `json:"results"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 || len(resp.Results) != 1 {
		t.Fatalf("wallet = %+v, want one entry", resp)
	}
	if resp.Results[0].Entitlement.Code != "WELCOME50" || !resp.Results[0].Eligible {
		t.Fatalf("entry = %+v, want eligible WELCOME50", resp.Results[0])
	}
}

func TestReservePromotionRequiresLedgerPermission(t *testing.T) {
	mux, svc := newPromotionHTTPMux(t)
	if _, err := svc.Create(t.Context(), fixedGlobalPromotion("SAVE5K", 5000)); err != nil {
		t.Fatalf("create: %v", err)
	}

	body := map[string]any{
		"code":             "SAVE5K",
		"order_id":         "ord-1",
		"customer_id":      "cust-1",
		"shipping_fee_won": 3000,
		"lines": []map[string]any{
			{"sku_id": "sku-1", "quantity": 1, "unit_price_won": 50000},
		},
	}
	token := makeAccessToken(t, "cust-1", nil)
	w := serve(t, mux, http.MethodPost, handler.RouteReservePromotion, token, body)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}

	serviceToken := makeAccessToken(t, "order-svc", []string{permissions.PromotionRedeem})
	w = serve(t, mux, http.MethodPost, handler.RouteReservePromotion, serviceToken, body)
	if w.Code != http.StatusOK {
		t.Fatalf("service reserve status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}
