package bootstrap

import (
	"github.com/elug3/dupli1/shared/pkg/settings"
)

// BuildSettings returns the public, non-secret settings payload for the product service.
// productViewsEnabled should mirror GuestCookieConfig.Enabled / PRODUCT_VIEWS_ENABLED.
func BuildSettings(cfg Config, productViewsEnabled bool) settings.Response {
	resp := settings.NewResponse("product")
	resp.Auth = settings.ConsumerAuth(cfg.JWKSURL, cfg.JWTSecret)
	resp.Storage = settings.StorageMode(cfg.DatabaseConnString)
	resp.Features = map[string]bool{
		"s3_images":        cfg.S3Endpoint != "",
		"nats_events":      cfg.NATSURL != "",
		"inventory_merged": true,
		"product_views":    productViewsEnabled,
		"recommendations":  true,
	}
	if cfg.S3Bucket != "" {
		resp.Limits = map[string]any{"s3_bucket": cfg.S3Bucket}
	}
	resp.Dependencies = map[string]settings.Dependency{
		"s3":   {Configured: cfg.S3Endpoint != ""},
		"nats": {Configured: cfg.NATSURL != ""},
	}
	return resp
}
