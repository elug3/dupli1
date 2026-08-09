package bootstrap

import (
	"github.com/elug3/dupli1/shared/pkg/money"
	"github.com/elug3/dupli1/shared/pkg/settings"
)

// BuildSettings returns the public, non-secret settings payload for the payment service.
func BuildSettings(cfg Config) settings.Response {
	resp := settings.NewResponse("payment")
	resp.Auth = settings.ConsumerAuth(cfg.JWKSURL, cfg.JWTSecret)
	resp.Storage = settings.StorageMode(cfg.DatabaseConnString)

	nanoOn := cfg.Nano.Enabled()
	devSimulate := cfg.AllowDevSimulate && !nanoOn
	checkoutProvider := "none"
	switch {
	case nanoOn:
		checkoutProvider = "nano"
	case devSimulate:
		checkoutProvider = "dev"
	}
	cardEnabled := nanoOn || devSimulate
	resp.Features = map[string]bool{
		"nats_events":          cfg.NATSURL != "",
		"nano_checkout":        nanoOn,
		"dev_simulate_success": devSimulate,
		"method_credit_card":   cardEnabled,
		"method_bypass":        true,
		"method_bitcoin":       false,
	}
	resp.Limits = map[string]any{
		"checkout_provider": checkoutProvider,
		"currency":          money.Currency, // only KRW; amount_cents is whole won
		"methods": map[string]bool{
			"credit_card": cardEnabled,
			"bypass":      true, // requires payment.bypass; storefront must hide
			"bitcoin":     false,
		},
	}
	if cfg.PublicBaseURL != "" {
		resp.Limits["public_base_url"] = cfg.PublicBaseURL
	}
	resp.Dependencies = map[string]settings.Dependency{
		"order": settings.Dep(cfg.OrderURL),
		"nats":  {Configured: cfg.NATSURL != ""},
	}
	return resp
}
