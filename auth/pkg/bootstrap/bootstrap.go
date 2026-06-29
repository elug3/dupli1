package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/elug3/schick/auth/pkg/handler"
	jwtinfra "github.com/elug3/schick/auth/pkg/infra/jwt"
	natsinfra "github.com/elug3/schick/auth/pkg/infra/nats"
	"github.com/elug3/schick/auth/pkg/infra/postgres"
	redisinfra "github.com/elug3/schick/auth/pkg/infra/redis"
	"github.com/elug3/schick/auth/pkg/ports"
	"github.com/elug3/schick/auth/pkg/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// App holds wired auth service dependencies and the HTTP router.
type App struct {
	Engine  *gin.Engine
	Handler *handler.Handler
	DB      *sql.DB
	Redis   *redis.Client
	close   func() error
}

// Close releases infrastructure resources opened during bootstrap.
func (a *App) Close() error {
	if a == nil || a.close == nil {
		return nil
	}
	return a.close()
}

// Bootstrap wires infrastructure, services, handlers, and HTTP routes.
func Bootstrap(ctx context.Context, cfg Config) (*App, error) {
	if cfg.TokenExpiry <= 0 {
		return nil, fmt.Errorf("token expiry must be > 0")
	}
	if cfg.RefreshTokenExpiry <= 0 {
		return nil, fmt.Errorf("refresh token expiry must be > 0")
	}

	accessTokenGen, refreshTokenGen, jwksJSON, err := buildTokenGenerators(cfg)
	if err != nil {
		return nil, err
	}

	db, err := openPostgres(ctx, cfg.DBURL, cfg.MaxConns, cfg.Logger)
	if err != nil {
		return nil, err
	}

	redisClient, err := openRedis(ctx, cfg.RedisURL)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	var eventPublisher ports.EventPublisher
	var natsPublisher *natsinfra.Publisher
	if cfg.NATSURL != "" {
		natsPublisher, err = natsinfra.NewPublisher(cfg.NATSURL)
		if err != nil {
			if redisClient != nil {
				_ = redisClient.Close()
			}
			_ = db.Close()
			return nil, err
		}
		eventPublisher = natsPublisher
	}

	if err := migrateSchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	userRepo := postgres.NewUserRepository(db)

	var sessionStore ports.SessionStore
	if redisClient != nil {
		sessionStore = redisinfra.NewSessionCache(redisClient)
	}

	svc := service.NewService(
		userRepo,
		accessTokenGen,
		service.WithRefreshTokenGen(refreshTokenGen, cfg.RefreshTokenExpiry),
		service.WithSessionStore(sessionStore),
		service.WithEventPublisher(eventPublisher),
	)

	h := handler.NewHandler(svc, cfg.Logger)
	engine := newRouter(h, cfg.Debug, jwksJSON, redisClient, cfg.CORSOrigins)

	app := &App{
		Engine:  engine,
		Handler: h,
		DB:      db,
		Redis:   redisClient,
		close: func() error {
			var errs []error
			if redisClient != nil {
				errs = append(errs, redisClient.Close())
			}
			if natsPublisher != nil {
				natsPublisher.Close()
			}
			errs = append(errs, db.Close())
			return errors.Join(errs...)
		},
	}

	if err := seedOwner(ctx, cfg, userRepo); err != nil {
		_ = app.Close()
		return nil, err
	}
	if err := seedWebServiceAccount(ctx, cfg, userRepo); err != nil {
		_ = app.Close()
		return nil, err
	}

	return app, nil
}

// buildTokenGenerators creates RS256 access and refresh token generators.
// When JWTPrivateKeyPEM is empty, an ephemeral 2048-bit RSA key is generated
// (dev mode only — tokens are invalidated on restart).
func buildTokenGenerators(cfg Config) (access ports.TokenGenerator, refresh ports.TokenGenerator, jwksJSON []byte, err error) {
	if len(cfg.JWTPrivateKeyPEM) > 0 {
		access, jwksJSON, err = newRSAGeneratorWithJWKS(cfg.JWTPrivateKeyPEM, cfg.JWTKeyID, int64(cfg.TokenExpiry.Seconds()), "access")
		if err != nil {
			return nil, nil, nil, err
		}
		refresh, _, err = newRSAGeneratorWithJWKS(cfg.JWTPrivateKeyPEM, cfg.JWTKeyID, int64(cfg.RefreshTokenExpiry.Seconds()), "refresh")
		if err != nil {
			return nil, nil, nil, err
		}
		return access, refresh, jwksJSON, nil
	}

	// No key configured — generate a throwaway RSA key. Tokens are invalid across restarts.
	cfg.Logger.Warn().Msg("no JWT_PRIVATE_KEY_FILE configured — generating ephemeral RSA-2048 key; tokens will be invalidated on restart")
	key, genErr := jwtinfra.GenerateRSAKey(2048)
	if genErr != nil {
		return nil, nil, nil, fmt.Errorf("generate ephemeral RSA key: %w", genErr)
	}
	rsaAccess := jwtinfra.NewRSATokenGeneratorWithType(key, cfg.JWTKeyID, int64(cfg.TokenExpiry.Seconds()), "access")
	rsaRefresh := jwtinfra.NewRSATokenGeneratorWithType(key, cfg.JWTKeyID, int64(cfg.RefreshTokenExpiry.Seconds()), "refresh")
	jwksJSON, err = json.Marshal(rsaAccess.PublicJWKS())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal JWKS: %w", err)
	}
	return rsaAccess, rsaRefresh, jwksJSON, nil
}

func newRSAGeneratorWithJWKS(pemBytes []byte, keyID string, expirySeconds int64, tokenType string) (*jwtinfra.RSATokenGenerator, []byte, error) {
	gen, err := jwtinfra.NewRSATokenGeneratorFromPEM(pemBytes, keyID, expirySeconds, tokenType)
	if err != nil {
		return nil, nil, err
	}
	jwksJSON, err := json.Marshal(gen.PublicJWKS())
	if err != nil {
		return nil, nil, fmt.Errorf("marshal JWKS: %w", err)
	}
	return gen, jwksJSON, nil
}
