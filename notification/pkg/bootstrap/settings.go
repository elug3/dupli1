package bootstrap

import (
	"github.com/elug3/dupli1/shared/pkg/settings"
)

// BuildSettings returns the public, non-secret settings payload for the notification service.
func BuildSettings(cfg Config) settings.Response {
	resp := settings.NewResponse("notification")
	resp.Features = map[string]bool{
		"nats_dispatcher":    cfg.NATSURL != "",
		"telegram_enabled":   cfg.TelegramToken != "",
		"order_chat_configured":   cfg.OrderChatID != "",
		"product_chat_configured": cfg.ProductChatID != "",
	}
	resp.Limits = map[string]any{
		"read_timeout_seconds":  settings.TimeoutSeconds(cfg.ReadTimeout),
		"write_timeout_seconds": settings.TimeoutSeconds(cfg.WriteTimeout),
	}
	resp.Dependencies = map[string]settings.Dependency{
		"nats":     {Configured: cfg.NATSURL != ""},
		"telegram": {Configured: cfg.TelegramToken != ""},
	}
	return resp
}
