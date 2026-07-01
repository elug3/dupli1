package bootstrap

import (
	"net/http"

	"github.com/elug3/dupli1/inventory/pkg/handler"
	"github.com/elug3/dupli1/inventory/pkg/infra/memory"
	"github.com/elug3/dupli1/inventory/pkg/ports"
	"github.com/elug3/dupli1/inventory/pkg/service"
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
