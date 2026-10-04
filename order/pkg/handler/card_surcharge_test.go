package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/elug3/dupli1/shared/pkg/permissions"
)

// A bypass order carries no card surcharge, so only someone who can then mark
// it paid by bypass may price one. Otherwise a shopper could skip the surcharge.
func TestCreateOrder_BypassMethodNeedsPaymentBypass(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)
	body := map[string]any{
		"customer_id":    "u-1",
		"items":          []map[string]any{{"sku": "ITEM-1", "quantity": 1}},
		"payment_method": "bypass",
	}

	w := do(t, mux, http.MethodPost, "/api/v1/orders", makeToken(t, "u-1", nil), body)
	if w.Code != http.StatusForbidden {
		t.Fatalf("shopper status = %d, want 403; body: %s", w.Code, w.Body.String())
	}

	w = do(t, mux, http.MethodPost, "/api/v1/orders", makeToken(t, "u-1", []string{permissions.PaymentBypass}), body)
	if w.Code != http.StatusCreated {
		t.Fatalf("staff status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	var order struct {
		PaymentMethod    string `json:"payment_method"`
		CardSurchargeWon int64  `json:"card_surcharge_won"`
	}
	if err := json.NewDecoder(w.Body).Decode(&order); err != nil {
		t.Fatal(err)
	}
	if order.PaymentMethod != "bypass" || order.CardSurchargeWon != 0 {
		t.Fatalf("method/surcharge = %q/%d, want bypass/0", order.PaymentMethod, order.CardSurchargeWon)
	}
}

func TestCreateOrder_UnknownMethodIs400(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)
	w := do(t, mux, http.MethodPost, "/api/v1/orders", makeToken(t, "u-1", nil), map[string]any{
		"customer_id":    "u-1",
		"items":          []map[string]any{{"sku": "ITEM-1", "quantity": 1}},
		"payment_method": "paypal",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}
