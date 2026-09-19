package bootstrap

import "time"

// Config holds everything Bootstrap needs to wire the support service.
type Config struct {
	Addr                  string
	DatabaseConnString    string
	TelegramToken         string
	TelegramWebhookURL    string
	TelegramWebhookSecret string
	TelegramAPIBase       string
	NATSURL               string
	JWTSecret             string
	JWKSURL               string
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	IdleTimeout           time.Duration
}
