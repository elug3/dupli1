package bootstrap

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/elug3/dupli1/payment/pkg/authjwt"
	"github.com/elug3/dupli1/payment/pkg/handler"
	"github.com/elug3/dupli1/payment/pkg/infra/checkout"
	"github.com/elug3/dupli1/payment/pkg/infra/httporder"
	"github.com/elug3/dupli1/payment/pkg/infra/memory"
	natsinfra "github.com/elug3/dupli1/payment/pkg/infra/nats"
	"github.com/elug3/dupli1/payment/pkg/infra/pg"
	"github.com/elug3/dupli1/payment/pkg/ports"
	"github.com/elug3/dupli1/payment/pkg/service"
)

type Config struct {
	OrderURL           string
	DatabaseConnString string
	JWTSecret          string
	JWKSURL            string
	NATSURL            string
	StripeSecretKey    string
	StripeWebhookSecret string
	StripeSuccessURL   string
	StripeCancelURL    string
	PublicBaseURL      string
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

	if cfg.OrderURL == "" {
		closeFn()
		return nil, fmt.Errorf("OrderURL is required")
	}
	orders := httporder.NewClient(cfg.OrderURL, cfg.HTTPClient)

	var checkoutProvider ports.CheckoutProvider
	if cfg.StripeSecretKey != "" {
		checkoutProvider = checkout.NewStripeProvider(cfg.StripeSecretKey, cfg.StripeSuccessURL, cfg.StripeCancelURL)
	} else {
		publicURL := cfg.PublicBaseURL
		if publicURL == "" {
			publicURL = "http://localhost:8080"
		}
		checkoutProvider = checkout.NewDevProvider(publicURL)
	}

	var eventPublisher ports.EventPublisher
	var natsPublisher *natsinfra.Publisher
	if cfg.NATSURL != "" {
		var err error
		natsPublisher, err = natsinfra.NewPublisher(cfg.NATSURL)
		if err != nil {
			closeFn()
			return nil, err
		}
		eventPublisher = natsPublisher
	}

	svc := service.New(repo, orders, checkoutProvider, eventPublisher)

	var jwtValidator handler.AccessTokenValidator
	if cfg.JWKSURL != "" || cfg.JWTSecret != "" {
		validator, err := authjwt.NewAccessTokenValidator(cfg.JWKSURL, cfg.JWTSecret)
		if err != nil {
			if natsPublisher != nil {
				natsPublisher.Close()
			}
			closeFn()
			return nil, fmt.Errorf("auth validator: %w", err)
		}
		jwtValidator = validator
	}

	h := handler.New(svc, jwtValidator, cfg.StripeWebhookSecret).WithSettings(BuildSettings(cfg))
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	closeAll := func() error {
		var errs []error
		if natsPublisher != nil {
			natsPublisher.Close()
		}
		errs = append(errs, closeFn())
		return errors.Join(errs...)
	}

	return &App{
		Router:  mux,
		Handler: h,
		Service: svc,
		close:   closeAll,
	}, nil
}

func openRepository(connString string) (ports.Repository, func() error, error) {
	if connString == "" {
		return memory.NewRepository(), func() error { return nil }, nil
	}

	pgRepo, err := pg.NewRepository(connString)
	if err != nil {
		return nil, nil, fmt.Errorf("payment repository: %w", err)
	}
	return pgRepo, func() error {
		pgRepo.Close()
		return nil
	}, nil
}

func DefaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}
