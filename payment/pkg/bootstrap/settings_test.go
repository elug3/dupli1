package bootstrap

import (
	"testing"

	"github.com/elug3/dupli1/payment/pkg/infra/checkout"
)

func TestBuildSettingsDevSimulateRequiresExplicitOptIn(t *testing.T) {
	cfg := Config{OrderURL: "http://order"}
	resp := BuildSettings(cfg)
	if resp.Features["dev_simulate_success"] {
		t.Fatal("dev_simulate_success should be false when AllowDevSimulate is unset")
	}
	if got := resp.Limits["checkout_provider"]; got != "none" {
		t.Fatalf("checkout_provider = %v, want none", got)
	}
	if resp.Features["method_credit_card"] {
		t.Fatal("method_credit_card should be false when simulate is off")
	}

	cfg.AllowDevSimulate = true
	resp = BuildSettings(cfg)
	if !resp.Features["dev_simulate_success"] {
		t.Fatal("dev_simulate_success should be true when AllowDevSimulate is set")
	}
	if got := resp.Limits["checkout_provider"]; got != "dev" {
		t.Fatalf("checkout_provider = %v, want dev", got)
	}
	if !resp.Features["method_credit_card"] {
		t.Fatal("method_credit_card should be true when simulate is on")
	}
}

func TestBuildSettingsNanoTakesPrecedence(t *testing.T) {
	cfg := Config{
		OrderURL:         "http://order",
		AllowDevSimulate: true,
		Nano: checkout.NanoConfig{
			ShopCode: "240000005",
			LoginID:  "shoptest",
			APIKey:   "test-key",
		},
	}
	resp := BuildSettings(cfg)
	if resp.Features["dev_simulate_success"] {
		t.Fatal("dev_simulate_success should be false when nano is configured")
	}
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
