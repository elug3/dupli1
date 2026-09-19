// Package bootstrap wires the support service: repositories, the router, the
// Telegram adapter, HTTP routes, and the inbound update path.
package bootstrap

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	tg "github.com/elug3/dupli1/shared/pkg/telegram"
	"github.com/elug3/dupli1/support/pkg/handler"
	"github.com/elug3/dupli1/support/pkg/infra/memory"
	telegraminfra "github.com/elug3/dupli1/support/pkg/infra/telegram"
	"github.com/elug3/dupli1/support/pkg/ports"
	"github.com/elug3/dupli1/support/pkg/service"
)

// App holds the wired service.
type App struct {
	Router        *http.ServeMux
	HTTP          *http.Server
	cancelWorkers context.CancelFunc
	close         func() error
}

// Close releases infrastructure resources opened during bootstrap.
func (a *App) Close() error {
	if a == nil {
		return nil
	}
	if a.cancelWorkers != nil {
		a.cancelWorkers()
	}
	if a.close == nil {
		return nil
	}
	return a.close()
}

// Bootstrap wires the support service and starts its inbound update path.
func Bootstrap(cfg Config) (*App, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("listen address is required")
	}
	// Fail closed at boot rather than at delivery time: SetWebhook without a
	// secret registers a webhook Telegram calls with no
	// X-Telegram-Bot-Api-Secret-Token header, which the handler then answers
	// with 503 for every update — a silently dead inbound path.
	if strings.TrimSpace(cfg.TelegramWebhookURL) != "" && strings.TrimSpace(cfg.TelegramWebhookSecret) == "" {
		return nil, fmt.Errorf("TELEGRAM_SUPPORT_WEBHOOK_SECRET is required when TELEGRAM_SUPPORT_WEBHOOK_URL is set")
	}

	conversations := openConversationRepository(cfg.DatabaseConnString)

	client := newBotClient(cfg)
	// No access policy is set, and that is the point: this bot answers whoever
	// writes to it. The ops bot's allowlist is the opposite posture and stays
	// with the ops bot.
	bot := &telegraminfra.Bot{Client: client}

	router := service.NewRouter(conversations, bot, newULID, time.Now)
	processor := &telegraminfra.UpdateProcessor{Router: router}

	// Long-lived worker root; cancelled on shutdown. Created before the handler
	// because acknowledged webhook updates are processed under it, past the
	// lifetime of their request.
	workerCtx, cancelWorkers := context.WithCancel(context.Background())

	h := handler.New(handler.Options{
		Updates:       processor,
		WebhookSecret: cfg.TelegramWebhookSecret,
		Settings:      BuildSettings(cfg),
		UpdateContext: workerCtx,
	})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	httpSrv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	if client.Enabled() {
		if err := startInbound(workerCtx, client, processor, cfg); err != nil {
			cancelWorkers()
			return nil, err
		}
	} else {
		log.Println("TELEGRAM_SUPPORT_BOT_TOKEN not set — the support bot is inert (HTTP still serves)")
	}

	return &App{
		Router:        mux,
		HTTP:          httpSrv,
		cancelWorkers: cancelWorkers,
		close:         func() error { return nil },
	}, nil
}

// newBotClient builds the Bot API client, honouring a mock API base when one
// is configured.
//
// A non-default base is logged loudly rather than silently obeyed: it means
// this process is not talking to Telegram, which is correct in a smoke test and
// a serious incident in production.
func newBotClient(cfg Config) *tg.Client {
	if base := strings.TrimSpace(cfg.TelegramAPIBase); base != "" {
		log.Printf("WARNING: support bot is using a non-default Telegram API base (%s) — local/dev only", base)
		return tg.NewClientWithAPIBase(cfg.TelegramToken, nil, base)
	}
	return tg.NewClient(cfg.TelegramToken, nil)
}

// startInbound registers the webhook, or falls back to long-polling when no
// webhook URL is configured — which is how local dev runs without a public URL.
func startInbound(ctx context.Context, client *tg.Client, processor tg.Handler, cfg Config) error {
	if cfg.TelegramWebhookURL == "" {
		go tg.RunPoller(ctx, client, processor)
		return nil
	}

	// Telegram answers getUpdates with 409 while a webhook is active, so the
	// backlog is drained with no webhook registered — including one left over
	// from a previous run.
	if err := client.DeleteWebhook(ctx); err != nil {
		log.Printf("support telegram deleteWebhook before drain: %v", err)
	}
	if err := tg.DrainUpdates(ctx, client, processor); err != nil {
		log.Printf("support telegram drain updates: %v", err)
	}
	// Menus are the whole point of this bot, and Telegram drops any update type
	// not named here — an unlisted button tap never arrives and the button
	// spins on the shopper's device forever.
	err := client.SetWebhook(ctx, cfg.TelegramWebhookURL, cfg.TelegramWebhookSecret,
		tg.WithAllowedUpdates("message", "callback_query"))
	if err != nil {
		return fmt.Errorf("set telegram webhook: %w", err)
	}
	log.Printf("support telegram webhook registered at %s", cfg.TelegramWebhookURL)
	return nil
}

// openConversationRepository picks storage. Postgres arrives in Phase 3; until
// then an unset DUPLI1_SUPPORT_DB is the only supported mode, and a set one is
// reported rather than silently ignored.
func openConversationRepository(connString string) ports.ConversationRepository {
	if strings.TrimSpace(connString) != "" {
		log.Println("DUPLI1_SUPPORT_DB is set but the Postgres repository lands in Phase 3 — using the in-memory store")
	}
	return memory.NewConversationRepository()
}

var ulidEntropy = ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0)

func newULID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy).String()
}
