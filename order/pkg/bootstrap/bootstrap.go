package bootstrap

import (
	"net/http"
	"time"

	"github.com/elug3/schick/order/pkg/authjwt"
	"github.com/elug3/schick/order/pkg/handler"
	"github.com/elug3/schick/order/pkg/infra/httpcoupon"
	"github.com/elug3/schick/order/pkg/infra/httpinventory"
	"github.com/elug3/schick/order/pkg/infra/memory"
	"github.com/elug3/schick/order/pkg/ports"
	"github.com/elug3/schick/order/pkg/service"
)

type Config struct {
	InventoryURL string
	ProductURL   string
	JWTSecret    string
	HTTPClient   *http.Client
}

type App struct {
	Router    *http.ServeMux
	Handler   *handler.Handler
	Service   *service.Service
	Repo      ports.Repository
	Inventory ports.InventoryClient
}

func Bootstrap(cfg Config) *App {
	repo := memory.NewRepository()
	inventory := httpinventory.NewClient(cfg.InventoryURL, cfg.HTTPClient)

	var couponClient ports.CouponClient
	if cfg.ProductURL != "" {
		couponClient = httpcoupon.NewClient(cfg.ProductURL, cfg.HTTPClient)
	}

	svc := service.NewWithCheckout(repo, inventory, couponClient, 0)

	var jwtValidator *authjwt.Validator
	if cfg.JWTSecret != "" {
		jwtValidator = authjwt.NewValidator(cfg.JWTSecret)
	}

	h := handler.New(svc, jwtValidator)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	return &App{
		Router:    mux,
		Handler:   h,
		Service:   svc,
		Repo:      repo,
		Inventory: inventory,
	}
}

func DefaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second}
}
