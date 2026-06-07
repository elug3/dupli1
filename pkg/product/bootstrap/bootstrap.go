package bootstrap

import (
	"context"
	"net/http"

	"github.com/schick/pkg/product/handler"
	"github.com/schick/pkg/product/infra/pg"
	"github.com/schick/pkg/product/service"
)

// App holds wired product service dependencies and the HTTP handler.
type App struct {
	Handler http.Handler
	close   func() error
}

// Close releases infrastructure resources opened during bootstrap.
func (a *App) Close() error {
	if a == nil || a.close == nil {
		return nil
	}
	return a.close()
}

// Bootstrap wires infrastructure, service, handler, and HTTP routes.
func Bootstrap(_ context.Context, cfg Config) (*App, error) {
	store, err := pg.NewProductStore(cfg.DatabaseConnString)
	if err != nil {
		return nil, err
	}

	svc := service.NewProductSearchService(store)
	h := handler.NewHandler(svc)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	return &App{
		Handler: mux,
		close: func() error {
			store.Close()
			return nil
		},
	}, nil
}
