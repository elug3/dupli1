package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	_ "github.com/lib/pq"

	"github.com/elug3/dupli1/profile/pkg/consumer"
	"github.com/elug3/dupli1/profile/pkg/handler"
	"github.com/elug3/dupli1/profile/pkg/infra/memory"
	natsinfra "github.com/elug3/dupli1/profile/pkg/infra/nats"
	"github.com/elug3/dupli1/profile/pkg/infra/postgres"
	"github.com/elug3/dupli1/profile/pkg/ports"
	"github.com/elug3/dupli1/profile/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/events"
	"github.com/elug3/dupli1/shared/pkg/pgsslmode"
)

// Config holds the dependencies Bootstrap needs to wire the profile service.
type Config struct {
	DatabaseConnString string
	JWTSecret          string
	JWKSURL            string
	NATSURL            string
}

// App holds wired profile service dependencies and the HTTP router.
type App struct {
	Router  *http.ServeMux
	Handler *handler.Handler
	Service *service.Service
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
func Bootstrap(ctx context.Context, cfg Config) (*App, error) {
	repo, closeRepo, err := openRepository(ctx, cfg.DatabaseConnString)
	if err != nil {
		return nil, err
	}

	svc := service.New(repo)

	jwtValidator, err := authjwt.NewAccessTokenValidator(cfg.JWKSURL, cfg.JWTSecret)
	if err != nil {
		_ = closeRepo()
		return nil, fmt.Errorf("auth validator: %w", err)
	}

	h := handler.New(svc, jwtValidator).WithSettings(BuildSettings(cfg))
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	var subscriber ports.EventSubscriber
	if cfg.NATSURL != "" {
		subscriber, err = natsinfra.NewSubscriber(cfg.NATSURL)
		if err != nil {
			_ = closeRepo()
			return nil, fmt.Errorf("profile nats subscriber: %w", err)
		}
		// Long-lived subscription root; cancelled only on process shutdown,
		// not tied to any single HTTP request.
		if err := subscriber.Subscribe(context.Background(), events.UserDeleted, consumer.HandleUserDeleted(svc)); err != nil {
			subscriber.Close()
			_ = closeRepo()
			return nil, fmt.Errorf("subscribe %s: %w", events.UserDeleted, err)
		}
	}

	return &App{
		Router:  mux,
		Handler: h,
		Service: svc,
		close: func() error {
			var errs []error
			if subscriber != nil {
				subscriber.Close()
			}
			errs = append(errs, closeRepo())
			return errors.Join(errs...)
		},
	}, nil
}

func openRepository(ctx context.Context, connString string) (ports.ProfileRepository, func() error, error) {
	if connString == "" {
		return memory.NewProfileRepository(), func() error { return nil }, nil
	}

	connString = pgsslmode.WithSSLMode(connString)
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return nil, nil, fmt.Errorf("open profile database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("ping profile database: %w", err)
	}
	if err := postgres.Migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, nil, err
	}

	repo := postgres.NewProfileRepository(db)
	return repo, db.Close, nil
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
