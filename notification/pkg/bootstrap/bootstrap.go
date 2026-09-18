package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/elug3/dupli1/notification/pkg/handler"
	"github.com/elug3/dupli1/notification/pkg/infra/memory"
	natsinfra "github.com/elug3/dupli1/notification/pkg/infra/nats"
	"github.com/elug3/dupli1/notification/pkg/infra/pg"
	telegraminfra "github.com/elug3/dupli1/notification/pkg/infra/telegram"
	"github.com/elug3/dupli1/notification/pkg/ports"
	"github.com/elug3/dupli1/notification/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
)

// App holds wired notification dependencies.
type App struct {
	HTTP          *http.Server
	subscriber    ports.EventSubscriber
	cancelWorkers context.CancelFunc
	close         func() error
}

// Close releases infrastructure resources.
func (a *App) Close() error {
	if a == nil || a.close == nil {
		return nil
	}
	return a.close()
}

// Bootstrap wires the HTTP server and Telegram notification dispatcher.
func Bootstrap(cfg Config) (*App, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("listen address is required")
	}

	// Fail closed at boot rather than at delivery time: SetWebhook without a
	// secret registers a webhook Telegram calls with no
	// X-Telegram-Bot-Api-Secret-Token header, which the handler then answers
	// with 503 for every update — a silently dead inbound path.
	if strings.TrimSpace(cfg.TelegramWebhookURL) != "" && strings.TrimSpace(cfg.TelegramWebhookSecret) == "" {
		return nil, fmt.Errorf("TELEGRAM_WEBHOOK_SECRET is required when TELEGRAM_WEBHOOK_URL is set")
	}

	envAllowlist := &ports.TelegramEnvAllowlist{
		OrderChatID:    cfg.OrderChatID,
		ProductChatID:  cfg.ProductChatID,
		AllowedUserIDs: cfg.AllowedUserIDs,
	}

	var closeFns []func() error
	telegramRepo, closeRepo, err := openTelegramRepository(cfg.DatabaseConnString)
	if err != nil {
		return nil, err
	}
	if closeRepo != nil {
		closeFns = append(closeFns, closeRepo)
	}

	telegramSubs := service.NewTelegramSubscriptions(telegramRepo)
	telegramAccess := service.NewTelegramAccess(telegramSubs, envAllowlist)
	refreshAccess := func() {
		// Process bootstrap: no HTTP request context available.
		if err := telegramAccess.Refresh(context.Background()); err != nil {
			log.Printf("telegram access refresh: %v", err)
		}
	}
	refreshAccess()

	notifier := telegraminfra.NewClient(cfg.TelegramToken, nil)
	notifier.SetAccessPolicy(telegramAccess)

	processor := &telegraminfra.UpdateProcessor{
		Client: notifier,
		Lookup: service.NewSubscriptionLookup(telegramSubs),
		Policy: telegramAccess,
	}

	var jwtValidator authjwt.AccessTokenValidator
	if cfg.JWKSURL != "" || cfg.JWTSecret != "" {
		jwtValidator, err = authjwt.NewAccessTokenValidator(cfg.JWKSURL, cfg.JWTSecret)
		if err != nil {
			return nil, fmt.Errorf("auth validator: %w", err)
		}
	}

	// Long-lived worker/subscriber root; cancelled on process shutdown. Created
	// before the handler because acknowledged webhook updates are processed
	// under it, past the lifetime of their request.
	telegramCtx, cancelWorkers := context.WithCancel(context.Background())

	// Keep this task's allowlist in step with accepts served by other tasks.
	go telegramAccess.RunRefresher(telegramCtx, cfg.AccessRefreshInterval)

	// Probed by /health. The NATS subscriber is created further down, so the
	// closure reads it when the probe runs rather than capturing a nil now.
	var natsSubscriber *natsinfra.Subscriber
	healthProbes := map[string]handler.HealthProbe{}
	if pinger, ok := telegramRepo.(interface {
		Ping(context.Context) error
	}); ok {
		healthProbes["postgres"] = pinger.Ping
	}
	if cfg.NATSURL != "" {
		healthProbes["nats"] = func(context.Context) error {
			if !natsSubscriber.Connected() {
				return fmt.Errorf("not connected to %s", cfg.NATSURL)
			}
			return nil
		}
	}

	settingsResp := BuildSettings(cfg, cfg.DatabaseConnString != "")
	h := handler.New(handler.Options{
		TelegramSubs:           telegramSubs,
		UpdateProcessor:        processor,
		WebhookSecret:          cfg.TelegramWebhookSecret,
		JWTValidator:           jwtValidator,
		Settings:               settingsResp,
		OnSubscriptionsChanged: refreshAccess,
		UpdateContext:          telegramCtx,
		HealthProbes:           healthProbes,
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

	var subscriber ports.EventSubscriber

	if notifier.Enabled() {
		if cfg.TelegramWebhookURL != "" {
			// Telegram answers getUpdates with 409 Conflict while a webhook is
			// active, so the backlog has to be drained with no webhook
			// registered — including one left over from a previous run.
			// Updates sent during this window are queued by Telegram and
			// arrive on the webhook once it is set.
			if err := notifier.DeleteWebhook(telegramCtx); err != nil {
				log.Printf("telegram deleteWebhook before drain: %v", err)
			}
			if err := telegraminfra.DrainUpdates(telegramCtx, notifier, processor); err != nil {
				log.Printf("telegram drain updates: %v", err)
			}
			if err := notifier.SetWebhook(telegramCtx, cfg.TelegramWebhookURL, cfg.TelegramWebhookSecret); err != nil {
				cancelWorkers()
				return nil, fmt.Errorf("set telegram webhook: %w", err)
			}
			log.Printf("telegram webhook registered at %s", cfg.TelegramWebhookURL)
		} else {
			go telegraminfra.RunPoller(telegramCtx, notifier, processor)
		}
	}

	if cfg.NATSURL != "" {
		natsSubscriber, err = natsinfra.NewSubscriber(cfg.NATSURL)
		if err != nil {
			cancelWorkers()
			return nil, err
		}
		subscriber = natsSubscriber
		closeFns = append(closeFns, func() error {
			natsSubscriber.Close()
			return nil
		})

		if !notifier.Enabled() {
			log.Println("TELEGRAM_BOT_TOKEN not set — Telegram messages will be skipped")
		}

		routing := service.NewTelegramRouting(telegramSubs, envAllowlist)
		dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
			Routing:       routing,
			OrderChatID:   cfg.OrderChatID,
			ProductChatID: cfg.ProductChatID,
			ManageWebURL:  cfg.ManageWebURL,
		})
		// Long-lived worker/subscriber root; cancelled on process shutdown.
		if err := dispatcher.Register(subscriber, context.Background()); err != nil {
			natsSubscriber.Close()
			cancelWorkers()
			return nil, err
		}
		log.Println("notification dispatcher subscribed to order and product events")
	} else {
		log.Println("NATS_URL not set — notification dispatcher disabled")
	}

	return &App{
		HTTP:          httpSrv,
		subscriber:    subscriber,
		cancelWorkers: cancelWorkers,
		close: func() error {
			cancelWorkers()
			var errs []error
			for _, fn := range closeFns {
				errs = append(errs, fn())
			}
			return errors.Join(errs...)
		},
	}, nil
}

func openTelegramRepository(connString string) (ports.TelegramRepository, func() error, error) {
	if connString == "" {
		return memory.NewTelegramRepository(), nil, nil
	}
	repo, err := pg.NewTelegramRepository(connString)
	if err != nil {
		return nil, nil, err
	}
	return repo, func() error {
		repo.Close()
		return nil
	}, nil
}
