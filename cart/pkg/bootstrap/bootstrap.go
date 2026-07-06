package bootstrap

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/elug3/dupli1/cart/pkg/authjwt"
	"github.com/elug3/dupli1/cart/pkg/handler"
	"github.com/elug3/dupli1/cart/pkg/infra/httpinventory"
	"github.com/elug3/dupli1/cart/pkg/infra/httpproduct"
	"github.com/elug3/dupli1/cart/pkg/infra/memory"
	"github.com/elug3/dupli1/cart/pkg/infra/pg"
	"github.com/elug3/dupli1/cart/pkg/ports"
	"github.com/elug3/dupli1/cart/pkg/service"
)

type Config struct {
	ProductURL         string
	InventoryURL       string
	DatabaseConnString string
	JWTSecret          string
	JWKSURL            string
	HTTPClient         *http.Client
}

type App struct {
	Router  *http.ServeMux
	Handler *handler.Handler
	Service *service.Service
	close   func() error
}

func (a *App) Close() error {
	if a == nil || a.close == nil {
		return nil
	}
	return a.close()
}

func Bootstrap(cfg Config) (*App, error) {
	repo, closeFn, err := openRepository(cfg.DatabaseConnString)
	if err != nil {
		return nil, err
	}

	var productClient ports.ProductClient
	if cfg.ProductURL != "" {
		productClient = httpproduct.NewClient(cfg.ProductURL, cfg.HTTPClient)
	}

	var inventoryClient ports.InventoryClient
	if cfg.InventoryURL != "" {
		inventoryClient = httpinventory.NewClient(cfg.InventoryURL, cfg.HTTPClient)
	}

	svc := service.New(repo, productClient, inventoryClient)

	var jwtValidator handler.AccessTokenValidator
	if cfg.JWKSURL != "" || cfg.JWTSecret != "" {
		validator, err := authjwt.NewAccessTokenValidator(cfg.JWKSURL, cfg.JWTSecret)
		if err != nil {
			closeFn()
			return nil, fmt.Errorf("auth validator: %w", err)
		}
		jwtValidator = validator
	}

	h := handler.New(svc, jwtValidator)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	return &App{
		Router:  mux,
		Handler: h,
		Service: svc,
		close:   closeFn,
	}, nil
}

func openRepository(connString string) (ports.Repository, func() error, error) {
	if connString == "" {
		return memory.NewRepository(), func() error { return nil }, nil
	}

	pgRepo, err := pg.NewRepository(connString)
	if err != nil {
		return nil, nil, fmt.Errorf("cart repository: %w", err)
	}
	return pgRepo, func() error {
		pgRepo.Close()
		return nil
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
