package bootstrap

import (
	"errors"
	"net/http"
	"time"

	natsinfra "github.com/elug3/dupli1/order/pkg/infra/nats"
	"github.com/elug3/dupli1/order/pkg/authjwt"
	"github.com/elug3/dupli1/order/pkg/handler"
	"github.com/elug3/dupli1/order/pkg/infra/httpcoupon"
	"github.com/elug3/dupli1/order/pkg/infra/httpinventory"
	"github.com/elug3/dupli1/order/pkg/infra/memory"
	"github.com/elug3/dupli1/order/pkg/ports"
	"github.com/elug3/dupli1/order/pkg/service"
)

type Config struct {
	InventoryURL string
	ProductURL   string
	JWTSecret    string
	NATSURL      string
	HTTPClient   *http.Client
}

type App struct {
	Router        *http.ServeMux
	Handler       *handler.Handler
	Service       *service.Service
	Repo          ports.Repository
	Inventory     ports.InventoryClient
	natsPublisher *natsinfra.Publisher
}

func (a *App) Close() error {
	if a == nil || a.natsPublisher == nil {
		return nil
	}
	a.natsPublisher.Close()
	return nil
}

func Bootstrap(cfg Config) (*App, error) {
	repo := memory.NewRepository()
	inventory := httpinventory.NewClient(cfg.InventoryURL, cfg.HTTPClient)

	var couponClient ports.CouponClient
	if cfg.ProductURL != "" {
		couponClient = httpcoupon.NewClient(cfg.ProductURL, cfg.HTTPClient)
	}

	var eventPublisher ports.EventPublisher
	var natsPublisher *natsinfra.Publisher
	if cfg.NATSURL != "" {
		var err error
		natsPublisher, err = natsinfra.NewPublisher(cfg.NATSURL)
		if err != nil {
			return nil, err
		}
		eventPublisher = natsPublisher
	}

	svc := service.NewWithCheckout(repo, inventory, couponClient, 0, eventPublisher)

	var jwtValidator *authjwt.Validator
	if cfg.JWTSecret != "" {
		jwtValidator = authjwt.NewValidator(cfg.JWTSecret)
	}

	h := handler.New(svc, jwtValidator)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	return &App{
		Router:        mux,
		Handler:       h,
		Service:       svc,
		Repo:          repo,
		Inventory:     inventory,
		natsPublisher: natsPublisher,
	}, nil
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
