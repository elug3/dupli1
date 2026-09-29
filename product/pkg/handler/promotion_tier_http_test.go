package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/handler"
)

// The storefront asks for the signed-in customer's own tier; the customer id
// comes from the token, so a body naming another account changes nothing.
func TestMyTierAnswersForTheTokenSubjectOnly(t *testing.T) {
	mux, svc := newPromotionHTTPMux(t)
	vip := domain.Promotion{
		Code: "VIP", Scope: domain.ScopeSingleUser, ApplyMode: domain.ApplyModeAuto, Active: true,
		Benefit: domain.Benefit{
			Target: domain.BenefitTargetGoods, DiscountType: domain.DiscountTypePercent,
			DiscountFraction: 0.10, ApplyTo: domain.ApplyToEntireSubtotal,
		},
	}
	if _, err := svc.Create(t.Context(), vip); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Issue(t.Context(), "VIP", "cust-vip", "issue", "manager:cust-vip", "mgr"); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	body := map[string]any{
		"customer_id": "cust-vip",
		"lines":       []map[string]any{{"sku_id": "sku-1", "quantity": 1, "unit_price_won": 100000}},
	}

	if w := serve(t, mux, http.MethodPost, handler.RouteMyTier, "", body); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", w.Code)
	}

	decode := func(token string) map[string]any {
		t.Helper()
		w := serve(t, mux, http.MethodPost, handler.RouteMyTier, token, body)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
		}
		var out map[string]any
		if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out
	}
	member := decode(makeAccessToken(t, "cust-vip", nil))
	if member["ok"] != true || member["code"] != "VIP" || member["discount_won"] != float64(10000) {
		t.Fatalf("member = %v, want ok VIP 10000", member)
	}
	other := decode(makeAccessToken(t, "cust-other", nil))
	if other["ok"] == true {
		t.Fatalf("another account read the member's tier: %v", other)
	}
}
