package order_test

import (
	"testing"

	order "github.com/elug3/dupli1/order/pkg"
)

func TestNewServerOptions_GatewayURLEmptyByDefault(t *testing.T) {
	opts := order.NewServerOptions()
	if opts.GatewayURL != "" {
		t.Fatalf("GatewayURL default = %q, want empty so DUPLI1_PRODUCT_URL is usable", opts.GatewayURL)
	}
	if opts.ProductURL == "" {
		t.Fatal("ProductURL default should remain set for local go run")
	}
}

// The configured default is the charge every deployment gets unless it opts
// out, so it is worth pinning: a silent change here re-prices every order.
func TestNewServerOptions_ShippingFeeDefault(t *testing.T) {
	opts := order.NewServerOptions()
	if opts.ShippingFeeWon != 0 {
		t.Fatalf("ShippingFeeWon default = %d, want 0 (free delivery)", opts.ShippingFeeWon)
	}
	if order.DefaultShippingFeeWon != 0 {
		t.Fatalf("DefaultShippingFeeWon = %d, want 0", order.DefaultShippingFeeWon)
	}
}
