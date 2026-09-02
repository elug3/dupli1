package bootstrap

import (
	"testing"

	"github.com/elug3/dupli1/payment/pkg/infra/checkout"
)

func TestBuildSettingsNoNanoMeansCardUnavailable(t *testing.T) {
	cfg := Config{OrderURL: "http://order"}
	resp := BuildSettings(cfg)
	if got := resp.Limits["checkout_provider"]; got != "none" {
		t.Fatalf("checkout_provider = %v, want none", got)
	}
	if resp.Features["method_credit_card"] {
		t.Fatal("method_credit_card should be false when nano is unset")
	}
	if !resp.Features["method_bypass"] {
		t.Fatal("method_bypass should always be true (permission-gated at request time)")
	}
}

func TestBuildSettingsNanoEnablesCard(t *testing.T) {
	cfg := Config{
		OrderURL: "http://order",
		Nano: checkout.NanoConfig{
			ShopCode: "240000005",
			LoginID:  "shoptest",
			APIKey:   "test-key",
		},
	}
	resp := BuildSettings(cfg)
	if !resp.Features["nano_checkout"] {
		t.Fatal("nano_checkout should be true")
	}
	if got := resp.Limits["checkout_provider"]; got != "nano" {
		t.Fatalf("checkout_provider = %v, want nano", got)
	}
	if !resp.Features["method_credit_card"] {
		t.Fatal("method_credit_card should be true with nano")
	}
}
