package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

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
	HTTP           *http.Server
	subscriber     ports.EventSubscriber
	cancelTelegram context.CancelFunc
	close          func() error
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

	settingsResp := BuildSettings(cfg, cfg.DatabaseConnString != "")
	h := handler.New(handler.Options{
		TelegramSubs:    telegramSubs,
		UpdateProcessor: processor,
		WebhookSecret:   cfg.TelegramWebhookSecret,
		JWTValidator:    jwtValidator,
		Settings:        settingsResp,
		OnSubscriptionsChanged: refreshAccess,
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
	var cancelTelegram context.CancelFunc

	if notifier.Enabled() {
		telegramCtx, cancel := context.WithCancel(context.Background())
		cancelTelegram = cancel

		if cfg.TelegramWebhookURL != "" {
			if err := notifier.SetWebhook(telegramCtx, cfg.TelegramWebhookURL, cfg.TelegramWebhookSecret); err != nil {
				cancel()
				return nil, fmt.Errorf("set telegram webhook: %w", err)
			}
			log.Printf("telegram webhook registered at %s", cfg.TelegramWebhookURL)
			if err := telegraminfra.DrainUpdates(telegramCtx, notifier, processor); err != nil {
				log.Printf("telegram drain updates: %v", err)
			}
		} else {
			go telegraminfra.RunPoller(telegramCtx, notifier, processor)
		}
	}

	if cfg.NATSURL != "" {
		natsSubscriber, err := natsinfra.NewSubscriber(cfg.NATSURL)
		if err != nil {
			if cancelTelegram != nil {
				cancelTelegram()
			}
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
		if err := dispatcher.Register(subscriber, context.Background()); err != nil {
			natsSubscriber.Close()
			if cancelTelegram != nil {
				cancelTelegram()
			}
			return nil, err
		}
		log.Println("notification dispatcher subscribed to order and product events")
	} else {
		log.Println("NATS_URL not set — notification dispatcher disabled")
	}

	return &App{
		HTTP:           httpSrv,
		subscriber:     subscriber,
		cancelTelegram: cancelTelegram,
		close: func() error {
			if cancelTelegram != nil {
				cancelTelegram()
			}
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
