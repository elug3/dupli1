package bootstrap

import (
	"github.com/elug3/dupli1/shared/pkg/money"
	"github.com/elug3/dupli1/shared/pkg/settings"
)

// BuildSettings returns the public, non-secret settings payload for the order service.
func BuildSettings(cfg Config) settings.Response {
	resp := settings.NewResponse("order")
	resp.Auth = settings.ConsumerAuth(cfg.JWKSURL, cfg.JWTSecret)
	resp.Storage = settings.StorageMode(cfg.DatabaseConnString)
	resp.Features = map[string]bool{
		"nats_events":           cfg.NATSURL != "",
		"payment_consumer":      cfg.NATSURL != "",
		"pending_expiry_worker": true,
		"checkout_sessions":     true,
	}
	resp.Limits = map[string]any{
		"currency": money.Currency, // *_cents amounts are whole KRW won
	}
	productURL := resolveProductURL(cfg)
	resp.Dependencies = map[string]settings.Dependency{
		"product": settings.Dep(productURL), // catalog, coupons, and stock/reservations
		"auth":    settings.Dep(cfg.AuthURL),
		"nats":    {Configured: cfg.NATSURL != ""},
	}
	return resp
}
