package bootstrap

import (
	"context"
	"log"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/elug3/dupli1/payment/pkg/handler"
	"github.com/elug3/dupli1/payment/pkg/infra/checkout"
	"github.com/elug3/dupli1/payment/pkg/infra/httporder"
	"github.com/elug3/dupli1/payment/pkg/infra/memory"
	natsinfra "github.com/elug3/dupli1/payment/pkg/infra/nats"
	"github.com/elug3/dupli1/payment/pkg/infra/pg"
	"github.com/elug3/dupli1/payment/pkg/ports"
	"github.com/elug3/dupli1/payment/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
)

type Config struct {
	OrderURL           string
	DatabaseConnString string
	JWTSecret          string
	JWKSURL            string
	NATSURL            string
	PublicBaseURL      string
	Nano               checkout.NanoConfig
	HTTPClient         *http.Client
}

type App struct {
	Router       *http.ServeMux
	Handler      *handler.Handler
	Service      *service.Service
	workerCancel context.CancelFunc
	close        func() error
}

func (a *App) Close() error {
	if a == nil {
		return nil
	}
	if a.workerCancel != nil {
		a.workerCancel()
	}
	if a.close == nil {
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

	nanoCfg := cfg.Nano
	if nanoCfg.PublicBaseURL == "" {
		nanoCfg.PublicBaseURL = cfg.PublicBaseURL
	}
	if nanoCfg.HTTPClient == nil {
		nanoCfg.HTTPClient = cfg.HTTPClient
	}
	var nanoProvider *checkout.NanoProvider
	var checkoutProvider ports.CheckoutProvider
	if nanoCfg.Enabled() {
		nanoProvider = checkout.NewNanoProvider(nanoCfg)
		checkoutProvider = nanoProvider
		if !nanoCfg.CallbackReachable() {
			// Card payments cannot complete in this state: NANO POSTs the
			// approval to receiveUrl from its own servers, and a loopback or
			// private base resolves to something on their side. Payments strand
			// at requires_payment with nothing in the logs, so say it once here.
			log.Printf(
				"payment: WARNING nano is configured but DUPLI1_PAYMENT_PUBLIC_URL=%q is not reachable from the internet; "+
					"NANO cannot deliver the approval callback and card payments will never leave requires_payment. "+
					"Set it to a publicly resolvable base URL (a tunnel is fine for local testing), or use method=bypass.",
				nanoCfg.PublicBaseURL,
			)
		}
	} else {
		checkoutProvider = checkout.NewUnavailableProvider(
			"card checkout is not configured; set NANO_* credentials or use method=bypass (payment.bypass)",
		)
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
	// Long-lived worker/subscriber root; cancelled on process shutdown.
	workerCtx, workerCancel := context.WithCancel(context.Background())
	svc.StartOutboxWorker(workerCtx, 2*time.Second)
	svc.StartReconcileWorker(workerCtx, 1*time.Minute, 2*time.Hour)

	jwtValidator, err := authjwt.NewAccessTokenValidator(cfg.JWKSURL, cfg.JWTSecret)
	if err != nil {
		workerCancel()
		if natsPublisher != nil {
			natsPublisher.Close()
		}
		closeFn()
		return nil, fmt.Errorf("auth validator: %w", err)
	}

	h := handler.New(svc, jwtValidator).
		WithSettings(BuildSettings(cfg))
	if nanoProvider != nil {
		h = h.WithNano(nanoProvider)
	}
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
		Router:       mux,
		Handler:      h,
		Service:      svc,
		workerCancel: workerCancel,
		close:        closeAll,
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
