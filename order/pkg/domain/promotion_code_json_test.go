package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/elug3/dupli1/order/pkg/domain"
)

// Both key spellings ship for one release so the storefront and this service
// can deploy in either order. See docs/product-promotion-rename.md.

func TestOrderEmitsBothPromotionCodeKeys(t *testing.T) {
	b, err := json.Marshal(domain.Order{ID: "ord-1", PromotionCode: "SUMMER30"})
	if err != nil {
		t.Fatalf("marshal order: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal order: %v", err)
	}
	if got["promotion_code"] != "SUMMER30" {
		t.Fatalf("promotion_code = %v, want SUMMER30", got["promotion_code"])
	}
	if got["coupon_code"] != "SUMMER30" {
		t.Fatalf("coupon_code = %v, want SUMMER30 (pre-rename readers)", got["coupon_code"])
	}
}

func TestCheckoutSessionEmitsBothPromotionCodeKeys(t *testing.T) {
	b, err := json.Marshal(domain.CheckoutSession{ID: "cs-1", PromotionCode: "SUMMER30"})
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal session: %v", err)
	}
	if got["promotion_code"] != "SUMMER30" || got["coupon_code"] != "SUMMER30" {
		t.Fatalf("session keys = %v / %v, want both SUMMER30", got["promotion_code"], got["coupon_code"])
	}
}

// An order with no promotional code must not gain empty keys under either name.
func TestNoPromotionCodeEmitsNeitherKey(t *testing.T) {
	b, err := json.Marshal(domain.Order{ID: "ord-2"})
	if err != nil {
		t.Fatalf("marshal order: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal order: %v", err)
	}
	if _, ok := got["promotion_code"]; ok {
		t.Fatal("promotion_code should be omitted when empty")
	}
	if _, ok := got["coupon_code"]; ok {
		t.Fatal("coupon_code should be omitted when empty")
	}
}
