package main

import (
	"flag"
	"testing"
)

func TestConfigureOptions_AppliesNanoEnv(t *testing.T) {
	t.Setenv("NANO_BASE_URL", "https://pay.nanopay.co.kr")
	t.Setenv("NANO_VER", "240000005")
	t.Setenv("NANO_SHOPCODE", "240000005")
	t.Setenv("NANO_LOGIN_ID", "shoptest")
	t.Setenv("NANO_API_KEY", "test-api-key")
	t.Setenv("NANO_SUCCESS_URL", "https://dupli1.com/checkout/success")
	t.Setenv("NANO_FAILURE_URL", "https://dupli1.com/checkout/failure")

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	opts, err := ConfigureOptions(fs, nil)
	if err != nil {
		t.Fatalf("ConfigureOptions: %v", err)
	}
	if opts.NanoBaseURL != "https://pay.nanopay.co.kr" {
		t.Fatalf("NanoBaseURL = %q", opts.NanoBaseURL)
	}
	if opts.NanoVer != "240000005" || opts.NanoShopCode != "240000005" {
		t.Fatalf("nano merchant ids = ver %q shop %q", opts.NanoVer, opts.NanoShopCode)
	}
	if opts.NanoLoginID != "shoptest" || opts.NanoAPIKey != "test-api-key" {
		t.Fatalf("nano credentials = login %q key %q", opts.NanoLoginID, opts.NanoAPIKey)
	}
	if opts.NanoSuccessURL == "" || opts.NanoFailureURL == "" {
		t.Fatalf("redirect urls missing: success=%q failure=%q", opts.NanoSuccessURL, opts.NanoFailureURL)
	}
}
