package bootstrap

import "time"

// Config holds notification service wiring configuration.
type Config struct {
	Addr                  string
	NATSURL               string
	DatabaseConnString    string
	JWTSecret             string
	JWKSURL               string
	TelegramToken         string
	TelegramWebhookURL    string
	TelegramWebhookSecret string
	AllowedUserIDs        string
	OrderChatID           string
	ProductChatID         string
	ManageWebURL          string
	// AccessRefreshInterval is how often the cached Telegram allowlist is
	// rebuilt from the database. Zero selects
	// service.DefaultAccessRefreshInterval; tests shorten it.
	AccessRefreshInterval time.Duration
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	IdleTimeout           time.Duration
	ShutdownTimeout       time.Duration
}
