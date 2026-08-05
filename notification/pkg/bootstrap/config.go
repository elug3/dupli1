package bootstrap

import "time"

// Config holds notification service wiring configuration.
type Config struct {
	Addr                   string
	NATSURL                string
	DatabaseConnString     string
	JWTSecret              string
	JWKSURL                string
	TelegramToken          string
	TelegramWebhookURL     string
	TelegramWebhookSecret  string
	AllowedUserIDs         string
	OrderChatID            string
	ProductChatID          string
	ManageWebURL           string
	ReadTimeout            time.Duration
	WriteTimeout           time.Duration
	IdleTimeout            time.Duration
	ShutdownTimeout        time.Duration
}
