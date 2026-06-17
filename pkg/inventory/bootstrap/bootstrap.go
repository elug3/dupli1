package bootstrap

import (
	"net/http"

	"github.com/elug3/schick/pkg/inventory/handler"
	"github.com/elug3/schick/pkg/inventory/infra/memory"
	"github.com/elug3/schick/pkg/inventory/ports"
	"github.com/elug3/schick/pkg/inventory/service"
)

type App struct {
	Router  *http.ServeMux
	Handler *handler.Handler
	Service *service.Service
	Repo    ports.Repository
}

func Bootstrap(repo ports.Repository) *App {
	if repo == nil {
		repo = memory.NewRepository()
	}

	svc := service.New(repo)
	h := handler.New(svc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	return &App{
		Router:  mux,
		Handler: h,
		Service: svc,
		Repo:    repo,
	}
}
