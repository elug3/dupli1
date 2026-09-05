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

// The broken state must be observable without container log access.
func TestBuildSettings_ReportsNanoCallbackReachability(t *testing.T) {
	nano := checkout.NanoConfig{Ver: "v", ShopCode: "s", LoginID: "l", APIKey: "k"}

	local := nano
	local.PublicBaseURL = "http://localhost:8080"
	if got := BuildSettings(Config{Nano: local}).Features["nano_callback_reachable"]; got {
		t.Fatal("localhost base must report the callback as unreachable")
	}

	public := nano
	public.PublicBaseURL = "https://pay.dupli1.com"
	if got := BuildSettings(Config{Nano: public}).Features["nano_callback_reachable"]; !got {
		t.Fatal("public base must report the callback as reachable")
	}

	// With nano off there is no callback to worry about.
	if got := BuildSettings(Config{}).Features["nano_callback_reachable"]; got {
		t.Fatal("nano disabled must not claim a reachable callback")
	}
}
