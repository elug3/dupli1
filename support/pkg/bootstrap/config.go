package bootstrap

import (
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"
)

// Config holds everything Bootstrap needs to wire the support service.
type Config struct {
	Addr                  string
	DatabaseConnString    string
	TelegramToken         string
	TelegramWebhookURL    string
	TelegramWebhookSecret string
	TelegramAPIBase       string
	NATSURL               string
	ManageWebURL          string
	BusinessHours         domain.BusinessHours
	JWTSecret             string
	JWKSURL               string
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	IdleTimeout           time.Duration
}
