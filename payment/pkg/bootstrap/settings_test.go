package bootstrap

import "testing"

func TestBuildSettingsDevSimulateRequiresExplicitOptIn(t *testing.T) {
	cfg := Config{OrderURL: "http://order"}
	resp := BuildSettings(cfg)
	if resp.Features["dev_simulate_success"] {
		t.Fatal("dev_simulate_success should be false when AllowDevSimulate is unset")
	}
	if got := resp.Limits["checkout_provider"]; got != "none" {
		t.Fatalf("checkout_provider = %v, want none", got)
	}

	cfg.AllowDevSimulate = true
	resp = BuildSettings(cfg)
	if !resp.Features["dev_simulate_success"] {
		t.Fatal("dev_simulate_success should be true when AllowDevSimulate is set and Stripe is empty")
	}
	if got := resp.Limits["checkout_provider"]; got != "dev" {
		t.Fatalf("checkout_provider = %v, want dev", got)
	}

	cfg.StripeSecretKey = "sk_test"
	resp = BuildSettings(cfg)
	if resp.Features["dev_simulate_success"] {
		t.Fatal("dev_simulate_success should be false when Stripe is configured")
	}
	if got := resp.Limits["checkout_provider"]; got != "stripe" {
		t.Fatalf("checkout_provider = %v, want stripe", got)
	}
}
