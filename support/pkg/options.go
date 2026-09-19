package support

import (
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"
)

// ServerOptions configures the support server process.
type ServerOptions struct {
	Addr                  string
	DatabaseConnString    string
	TelegramToken         string
	TelegramWebhookURL    string
	TelegramWebhookSecret string
	TelegramAPIBase       string
	NATSURL               string
	ManageWebURL          string
	BusinessHours         domain.BusinessHours
	MessageRetention      time.Duration
	JWTSecret             string
	JWKSURL               string
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	IdleTimeout           time.Duration
	ShutdownTimeout       time.Duration
}

func NewServerOptions() *ServerOptions {
	return &ServerOptions{
		Addr:            ":8089",
		BusinessHours:   domain.DefaultBusinessHours(),
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    10 * time.Second,
		IdleTimeout:     120 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}
}
