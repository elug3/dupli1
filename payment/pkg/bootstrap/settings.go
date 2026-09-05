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
	checkoutProvider := "none"
	if nanoOn {
		checkoutProvider = "nano"
	}
	resp.Features = map[string]bool{
		"nats_events":   cfg.NATSURL != "",
		"nano_checkout": nanoOn,
		// False when nano is on but DUPLI1_PAYMENT_PUBLIC_URL is loopback or
		// private: NANO cannot deliver the approval callback, so card payments
		// never leave requires_payment. Surfaced here so the broken state is
		// observable without shell access to the container logs.
		"nano_callback_reachable": nanoOn && cfg.Nano.CallbackReachable(),
		"method_credit_card":      nanoOn,
		"method_bypass":           true,
		"method_bitcoin":          false,
	}
	resp.Limits = map[string]any{
		"checkout_provider": checkoutProvider,
		"currency":          money.Currency, // only KRW; amount_cents is whole won
		"methods": map[string]bool{
			"credit_card": nanoOn,
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
