package bootstrap

import "github.com/elug3/dupli1/shared/pkg/settings"

// BuildSettings returns the public, non-secret settings payload for the
// support service. Never a token, never a DSN — only whether one is set.
func BuildSettings(cfg Config) settings.Response {
	resp := settings.NewResponse("support")
	resp.Auth = settings.ConsumerAuth(cfg.JWKSURL, cfg.JWTSecret)
	resp.Storage = settings.StorageMode(cfg.DatabaseConnString)
	resp.Features = map[string]bool{
		"telegram_bot":     cfg.TelegramToken != "",
		"telegram_webhook": cfg.TelegramWebhookURL != "",
	}
	resp.Dependencies = map[string]settings.Dependency{
		"nats": settings.Dep(cfg.NATSURL),
	}
	return resp
}
