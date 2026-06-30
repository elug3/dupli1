package bootstrap

import "time"

// Config holds notification service wiring configuration.
type Config struct {
	Addr            string
	NATSURL         string
	TelegramToken   string
	OrderChatID     string
	ProductChatID   string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}
