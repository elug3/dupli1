package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	natsinfra "github.com/elug3/dupli1/notification/pkg/infra/nats"
	telegraminfra "github.com/elug3/dupli1/notification/pkg/infra/telegram"
	"github.com/elug3/dupli1/notification/pkg/ports"
	"github.com/elug3/dupli1/notification/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/settings"
)

// App holds wired notification dependencies.
type App struct {
	HTTP       *http.Server
	subscriber ports.EventSubscriber
	close      func() error
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

	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)
	mux.HandleFunc("/api/v1/notification/health", health)
	settingsResp := BuildSettings(cfg)
	mux.HandleFunc("/settings", settings.Handler(settingsResp))
	mux.HandleFunc("/api/v1/notification/settings", settings.Handler(settingsResp))

	httpSrv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	var subscriber ports.EventSubscriber
	var closeFns []func() error

	if cfg.NATSURL != "" {
		natsSubscriber, err := natsinfra.NewSubscriber(cfg.NATSURL)
		if err != nil {
			return nil, err
		}
		subscriber = natsSubscriber
		closeFns = append(closeFns, func() error {
			natsSubscriber.Close()
			return nil
		})

		notifier := telegraminfra.NewClient(cfg.TelegramToken, nil)
		if !notifier.Enabled() {
			log.Println("TELEGRAM_BOT_TOKEN not set — Telegram messages will be skipped")
		}

		dispatcher := service.NewDispatcher(notifier, service.DispatcherConfig{
			OrderChatID:   cfg.OrderChatID,
			ProductChatID: cfg.ProductChatID,
		})
		if err := dispatcher.Register(subscriber, context.Background()); err != nil {
			natsSubscriber.Close()
			return nil, err
		}
		log.Println("notification dispatcher subscribed to order and product events")
	} else {
		log.Println("NATS_URL not set — notification dispatcher disabled")
	}

	return &App{
		HTTP:       httpSrv,
		subscriber: subscriber,
		close: func() error {
			var errs []error
			for _, fn := range closeFns {
				errs = append(errs, fn())
			}
			return errors.Join(errs...)
		},
	}, nil
}

func health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
