package bootstrap

import (
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"
)

// DefaultInquiryQuietPeriod is how long an inquiry may sit untouched before
// the sweeper closes it.
const DefaultInquiryQuietPeriod = 7 * 24 * time.Hour

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
	// InquiryQuietPeriod is how long an inquiry may sit untouched before it
	// is closed automatically. Zero means DefaultInquiryQuietPeriod.
	InquiryQuietPeriod time.Duration
	JWTSecret          string
	JWKSURL            string
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleTimeout        time.Duration
}
