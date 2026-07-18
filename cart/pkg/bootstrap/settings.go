package bootstrap

import (
	"github.com/elug3/dupli1/shared/pkg/money"
	"github.com/elug3/dupli1/shared/pkg/settings"
)

// BuildSettings returns the public, non-secret settings payload for the cart service.
func BuildSettings(cfg Config) settings.Response {
	resp := settings.NewResponse("cart")
	resp.Auth = settings.ConsumerAuth(cfg.JWKSURL, cfg.JWTSecret)
	resp.Storage = settings.StorageMode(cfg.DatabaseConnString)
	resp.Features = map[string]bool{
		"product_enrichment":   cfg.ProductURL != "",
		"inventory_enrichment": cfg.InventoryURL != "",
	}
	resp.Limits = map[string]any{
		"currency": money.Currency, // unit_price_cents / subtotal_cents are whole KRW won
	}
	resp.Dependencies = map[string]settings.Dependency{
		"product":   settings.Dep(cfg.ProductURL),
		"inventory": settings.Dep(cfg.InventoryURL),
	}
	return resp
}
