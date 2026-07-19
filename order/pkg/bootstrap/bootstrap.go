package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	natsinfra "github.com/elug3/dupli1/order/pkg/infra/nats"
	"github.com/elug3/dupli1/order/pkg/authjwt"
	"github.com/elug3/dupli1/order/pkg/handler"
	"github.com/elug3/dupli1/order/pkg/infra/httpauth"
	"github.com/elug3/dupli1/order/pkg/infra/httpcoupon"
	"github.com/elug3/dupli1/order/pkg/infra/httpinventory"
	"github.com/elug3/dupli1/order/pkg/infra/memory"
	"github.com/elug3/dupli1/order/pkg/infra/pg"
	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/elug3/dupli1/order/pkg/service"
)

type Config struct {
	InventoryURL             string
	ProductURL               string
	AuthURL                  string
	InventoryServiceEmail    string
	InventoryServicePassword string
	InventoryBearerToken     string
	DatabaseConnString       string
	JWTSecret                string
	JWKSURL                  string
	NATSURL                  string
	HTTPClient               *http.Client
}

type App struct {
	Router        *http.ServeMux
	Handler       *handler.Handler
	Service       *service.Service
	Repo          ports.Repository
	Inventory     ports.InventoryClient
	natsPublisher *natsinfra.Publisher
	natsSubscriber *natsinfra.Subscriber
	expiryCancel    context.CancelFunc
	close         func() error
}

func (a *App) Close() error {
	if a == nil {
		return nil
	}
	var errs []error
	if a.expiryCancel != nil {
		a.expiryCancel()
	}
	if a.natsSubscriber != nil {
		a.natsSubscriber.Close()
	}
	if a.natsPublisher != nil {
		a.natsPublisher.Close()
	}
	if a.close != nil {
		errs = append(errs, a.close())
	}
	return errors.Join(errs...)
}

func Bootstrap(cfg Config) (*App, error) {
	repo, closeFn, err := openRepository(cfg.DatabaseConnString)
	if err != nil {
		return nil, err
	}
	inventoryTokenSource, err := resolveInventoryTokenSource(context.Background(), cfg)
	if err != nil {
		closeFn()
		return nil, err
	}
	inventory := httpinventory.NewClient(cfg.InventoryURL, cfg.HTTPClient, inventoryTokenSource)

	var couponClient ports.CouponClient
	if cfg.ProductURL != "" {
		couponClient = httpcoupon.NewClient(cfg.ProductURL, cfg.HTTPClient)
	}

	var eventPublisher ports.EventPublisher
	var natsPublisher *natsinfra.Publisher
	var natsSubscriber *natsinfra.Subscriber
	if cfg.NATSURL != "" {
		var err error
		natsPublisher, err = natsinfra.NewPublisher(cfg.NATSURL)
		if err != nil {
			return nil, err
		}
		eventPublisher = natsPublisher

		natsSubscriber, err = natsinfra.NewSubscriber(cfg.NATSURL)
		if err != nil {
			natsPublisher.Close()
			return nil, err
		}
	}

	svc := service.NewWithCheckout(repo, inventory, couponClient, 0, eventPublisher)

	if natsSubscriber != nil {
		if err := svc.RegisterPaymentConsumer(context.Background(), natsSubscriber); err != nil {
			natsSubscriber.Close()
			natsPublisher.Close()
			closeFn()
			return nil, fmt.Errorf("payment consumer: %w", err)
		}
	}
	expiryCtx, expiryCancel := context.WithCancel(context.Background())
	svc.StartPendingExpiryWorker(expiryCtx, 30*time.Second)

	var jwtValidator handler.AccessTokenValidator
	if cfg.JWKSURL != "" || cfg.JWTSecret != "" {
		validator, err := authjwt.NewAccessTokenValidator(cfg.JWKSURL, cfg.JWTSecret)
		if err != nil {
			return nil, fmt.Errorf("auth validator: %w", err)
		}
		jwtValidator = validator
	}

	h := handler.New(svc, jwtValidator).WithSettings(BuildSettings(cfg))
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	return &App{
		Router:         mux,
		Handler:        h,
		Service:        svc,
		Repo:           repo,
		Inventory:      inventory,
		natsPublisher:  natsPublisher,
		natsSubscriber: natsSubscriber,
		expiryCancel:   expiryCancel,
		close:          closeFn,
	}, nil
}

func openRepository(connString string) (ports.Repository, func() error, error) {
	if connString == "" {
		return memory.NewRepository(), func() error { return nil }, nil
	}

	pgRepo, err := pg.NewRepository(connString)
	if err != nil {
		return nil, nil, fmt.Errorf("order repository: %w", err)
	}
	return pgRepo, func() error {
		pgRepo.Close()
		return nil
	}, nil
}

func resolveInventoryTokenSource(ctx context.Context, cfg Config) (httpauth.TokenSource, error) {
	if cfg.InventoryBearerToken != "" {
		return httpauth.StaticToken(cfg.InventoryBearerToken), nil
	}
	if cfg.AuthURL == "" || cfg.InventoryServiceEmail == "" || cfg.InventoryServicePassword == "" {
		return nil, nil
	}
	src := httpauth.NewServiceAccountTokenSource(cfg.AuthURL, cfg.InventoryServiceEmail, cfg.InventoryServicePassword, cfg.HTTPClient)
	// Prime the cache at startup so misconfigured credentials fail fast.
	if _, err := src.Token(ctx); err != nil {
		return nil, fmt.Errorf("inventory service account token: %w", err)
	}
	return src, nil
}

func DefaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second}
}

func CloseApps(apps ...*App) error {
	var errs []error
	for _, app := range apps {
		if app != nil {
			errs = append(errs, app.Close())
		}
	}
	return errors.Join(errs...)
}
