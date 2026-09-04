package bootstrap

import (
	"github.com/elug3/dupli1/shared/pkg/settings"
)

// BuildSettings returns the public, non-secret settings payload for the
// profile service.
func BuildSettings(cfg Config) settings.Response {
	resp := settings.NewResponse("profile")
	resp.Auth = settings.ConsumerAuth(cfg.JWKSURL, cfg.JWTSecret)
	resp.Storage = settings.StorageMode(cfg.DatabaseConnString)
	resp.Features = map[string]bool{
		"user_deleted_consumer": cfg.NATSURL != "",
	}
	resp.Dependencies = map[string]settings.Dependency{
		"nats": settings.Dep(cfg.NATSURL),
	}
	return resp
}
