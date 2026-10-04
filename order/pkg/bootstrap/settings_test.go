package bootstrap_test

import (
	"testing"

	"github.com/elug3/dupli1/order/pkg/bootstrap"
)

// Storefronts read the delivery charge from here rather than hardcoding it, so
// the figure they quote matches what the service charges.
func TestBuildSettings_PublishesShippingFee(t *testing.T) {
	resp := bootstrap.BuildSettings(bootstrap.Config{ShippingFeeWon: 30000})
	got, ok := resp.Limits["shipping_fee_won"]
	if !ok {
		t.Fatalf("limits missing shipping_fee_won: %+v", resp.Limits)
	}
	if got != int64(30000) {
		t.Fatalf("shipping_fee_won = %v (%T), want int64 30000", got, got)
	}

	// Free delivery must publish an explicit 0, not vanish from the payload —
	// an absent field would send storefronts back to their own default.
	free := bootstrap.BuildSettings(bootstrap.Config{})
	if v, ok := free.Limits["shipping_fee_won"]; !ok || v != int64(0) {
		t.Fatalf("free delivery: got %v (present=%t), want 0", v, ok)
	}
}

// Storefronts quote the card surcharge line from this rate.
func TestBuildSettings_PublishesCardSurcharge(t *testing.T) {
	resp := bootstrap.BuildSettings(bootstrap.Config{CardSurchargeBps: 1000})
	if v := resp.Limits["card_surcharge_bps"]; v != int64(1000) {
		t.Fatalf("card_surcharge_bps = %v (%T), want int64 1000", v, v)
	}
	if v, ok := bootstrap.BuildSettings(bootstrap.Config{}).Limits["card_surcharge_bps"]; !ok || v != int64(0) {
		t.Fatalf("no surcharge: got %v (present=%t), want an explicit 0", v, ok)
	}
}
