package bootstrap

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/elug3/dupli1/inventory/pkg/handler"
	"github.com/elug3/dupli1/inventory/pkg/infra/memory"
	"github.com/elug3/dupli1/inventory/pkg/infra/pg"
	"github.com/elug3/dupli1/inventory/pkg/ports"
	"github.com/elug3/dupli1/inventory/pkg/service"
)

type Config struct {
	DatabaseConnString string
}

type App struct {
	Router  *http.ServeMux
	Handler *handler.Handler
	Service *service.Service
	Repo    ports.Repository
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

	svc := service.New(repo)
	h := handler.New(svc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	return &App{
		Router:  mux,
		Handler: h,
		Service: svc,
		Repo:    repo,
		close:   closeFn,
	}, nil
}

func openRepository(connString string) (ports.Repository, func() error, error) {
	if connString == "" {
		return memory.NewRepository(), func() error { return nil }, nil
	}

	pgRepo, err := pg.NewRepository(connString)
	if err != nil {
		return nil, nil, fmt.Errorf("inventory repository: %w", err)
	}
	return pgRepo, func() error {
		pgRepo.Close()
		return nil
	}, nil
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
