package bootstrap

import (
	"github.com/elug3/dupli1/shared/pkg/settings"
)

// BuildSettings returns the public, non-secret settings payload for the auth service.
func BuildSettings(cfg Config) settings.Response {
	resp := settings.NewResponse("auth")
	ephemeralKey := len(cfg.JWTPrivateKeyPEM) == 0
	kid := cfg.JWTKeyID
	if kid == "" {
		kid = "default"
	}
	resp.Auth = &settings.AuthInfo{
		Mode:           "rs256",
		JWKSConfigured: true,
	}
	resp.Storage = "postgres"
	resp.Features = map[string]bool{
		"redis_rate_limits": cfg.RedisURL != "",
		"nats_events":       cfg.NATSURL != "",
		"debug":             cfg.Debug,
		"ephemeral_jwt_key": ephemeralKey,
		"cors_enabled":      len(cfg.CORSOrigins) > 0,
	}
	resp.Limits = map[string]any{
		"token_expiry_seconds":         settings.TimeoutSeconds(cfg.TokenExpiry),
		"refresh_token_expiry_seconds": settings.TimeoutSeconds(cfg.RefreshTokenExpiry),
		"max_conns":                    cfg.MaxConns,
		"jwt_key_id":                   kid,
		"cors_origins_count":           len(cfg.CORSOrigins),
	}
	resp.Dependencies = map[string]settings.Dependency{
		"postgres": {Configured: cfg.DBURL != ""},
		"redis":    {Configured: cfg.RedisURL != ""},
		"nats":     {Configured: cfg.NATSURL != ""},
	}
	return resp
}
