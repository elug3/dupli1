package bootstrap

import (
	"net/http"
	"time"

	"github.com/elug3/schick/pkg/order/handler"
	"github.com/elug3/schick/pkg/order/infra/httpcoupon"
	"github.com/elug3/schick/pkg/order/infra/httpinventory"
	"github.com/elug3/schick/pkg/order/infra/memory"
	"github.com/elug3/schick/pkg/order/ports"
	"github.com/elug3/schick/pkg/order/service"
)

type Config struct {
	InventoryURL string
	ProductURL   string
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
	h := handler.New(svc)
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
