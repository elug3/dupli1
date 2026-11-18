package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/elug3/dupli1/product/pkg/handler"
)

// The retired tier route stays for one release and always says no tier
// applies, still only to a signed-in caller.
func TestMyTierAnswersNoTier(t *testing.T) {
	mux, _ := newPromotionHTTPMux(t)
	body := map[string]any{
		"lines": []map[string]any{{"sku_id": "sku-1", "quantity": 1, "unit_price_won": 100000}},
	}
	if w := serve(t, mux, http.MethodPost, handler.RouteMyTier, "", body); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", w.Code)
	}
	w := serve(t, mux, http.MethodPost, handler.RouteMyTier, makeAccessToken(t, "cust-1", nil), body)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["ok"] != false {
		t.Fatalf("got %v, want ok false", out)
	}
}
