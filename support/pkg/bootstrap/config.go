package bootstrap

import (
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"
)

// DefaultInquiryQuietPeriod is how long an inquiry may sit untouched before
// the sweeper closes it.
const DefaultInquiryQuietPeriod = 7 * 24 * time.Hour

// DefaultMessageRetention is how long a transcript keeps its words. Decided in
// docs/support-telegram-bot.md; the inquiry's metadata outlives it.
const DefaultMessageRetention = 180 * 24 * time.Hour

// DefaultReplyNoticeDelay is how long a web reply may go unread before the
// shopper gets an email about it.
const DefaultReplyNoticeDelay = 5 * time.Minute

// SMTPConfig is the reply-notice mail relay.
type SMTPConfig struct {
	Addr     string
	Username string
	Password string
	From     string
}

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
	// MessageRetention is how long message bodies are kept. Zero means
	// DefaultMessageRetention; negative disables the purge.
	MessageRetention time.Duration
	// GatewayURL is the internal nginx gateway (DUPLI1_GATEWAY_URL), through
	// which web chat resolves product and order references. Empty refuses
	// references with 503 but still serves text.
	GatewayURL string
	// StorefrontURL is the storefront's origin; a reply notice links to its
	// chat page. Empty disables reply notices.
	StorefrontURL string
	// SMTP sends the shopper's reply notice. An empty Addr disables it.
	SMTP SMTPConfig
	// ReplyNoticeDelay is how long a web reply may sit unread before the
	// shopper is emailed. Zero means DefaultReplyNoticeDelay.
	ReplyNoticeDelay time.Duration
	JWTSecret        string
	JWKSURL          string
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	IdleTimeout      time.Duration
}
