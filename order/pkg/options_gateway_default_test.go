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
