package bootstrap_test

import (
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/notification/pkg/bootstrap"
)

func baseConfig() bootstrap.Config {
	return bootstrap.Config{
		Addr:         ":0",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  5 * time.Second,
	}
}

// A webhook URL without a secret used to boot cleanly and then answer 503 to
// every update Telegram delivered, because the handler is fail-closed on a
// missing secret. Refuse the configuration at startup instead.
func TestBootstrapRequiresWebhookSecret(t *testing.T) {
	cfg := baseConfig()
	cfg.TelegramWebhookURL = "https://example.com/api/v1/notification/telegram/webhook"

	app, err := bootstrap.Bootstrap(cfg)
	if err == nil {
		t.Cleanup(func() { _ = app.Close() })
		t.Fatal("expected bootstrap to reject a webhook URL with no secret")
	}
	if !strings.Contains(err.Error(), "TELEGRAM_WEBHOOK_SECRET") {
		t.Fatalf("error should name the missing variable, got: %v", err)
	}
}

// Without a webhook URL the secret is irrelevant — polling mode must still boot.
func TestBootstrapAllowsPollingWithoutWebhookSecret(t *testing.T) {
	app, err := bootstrap.Bootstrap(baseConfig())
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
}
