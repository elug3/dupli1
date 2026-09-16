package domain

import "encoding/json"

// Order and CheckoutSession emit the promotion code under both names for one
// release: promotion_code (canonical) and coupon_code (pre-rename). Emitting
// both is what makes the deploy order between this service and the frontends
// irrelevant — a client reading either key keeps working.
//
// Nothing decodes coupon_code here: the code only ever arrives as {"code": …}
// on the apply route, so this is a write-side compatibility shim. Drop both
// MarshalJSON methods, and the legacy key, once the clients have moved.
// See docs/product-promotion-rename.md.

func (o Order) MarshalJSON() ([]byte, error) {
	type alias Order
	return json.Marshal(struct {
		alias
		LegacyPromotionCode string `json:"coupon_code,omitempty"`
	}{
		alias:               alias(o),
		LegacyPromotionCode: o.PromotionCode,
	})
}

func (s CheckoutSession) MarshalJSON() ([]byte, error) {
	type alias CheckoutSession
	return json.Marshal(struct {
		alias
		LegacyPromotionCode string `json:"coupon_code,omitempty"`
	}{
		alias:               alias(s),
		LegacyPromotionCode: s.PromotionCode,
	})
}
