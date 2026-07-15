package bootstrap

import (
	"github.com/elug3/dupli1/shared/pkg/settings"
)

// BuildSettings returns the public, non-secret settings payload for the payment service.
func BuildSettings(cfg Config) settings.Response {
	resp := settings.NewResponse("payment")
	resp.Auth = settings.ConsumerAuth(cfg.JWKSURL, cfg.JWTSecret)
	resp.Storage = settings.StorageMode(cfg.DatabaseConnString)

	checkoutProvider := "dev"
	if cfg.StripeSecretKey != "" {
		checkoutProvider = "stripe"
	}
	resp.Features = map[string]bool{
		"nats_events":               cfg.NATSURL != "",
		"stripe_checkout":           cfg.StripeSecretKey != "",
		"stripe_webhook":            cfg.StripeWebhookSecret != "",
		"dev_simulate_success":      cfg.StripeSecretKey == "",
	}
	// Expose provider as a dependency-style flag via features; also mirror in limits for clarity.
	resp.Limits = map[string]any{
		"checkout_provider": checkoutProvider,
	}
	if cfg.PublicBaseURL != "" {
		resp.Limits["public_base_url"] = cfg.PublicBaseURL
	}
	if cfg.StripeSuccessURL != "" {
		resp.Limits["stripe_success_url"] = cfg.StripeSuccessURL
	}
	if cfg.StripeCancelURL != "" {
		resp.Limits["stripe_cancel_url"] = cfg.StripeCancelURL
	}
	resp.Dependencies = map[string]settings.Dependency{
		"order": settings.Dep(cfg.OrderURL),
		"nats":  {Configured: cfg.NATSURL != ""},
	}
	return resp
}
