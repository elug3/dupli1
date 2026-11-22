package main

import (
	auth "github.com/elug3/dupli1/auth/pkg"
	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/shared/pkg/natsauth"
	"github.com/elug3/dupli1/shared/pkg/runenv"
)

// checkProd refuses a production start (DUPLI1_ENV unset or prod) that would
// fall back on an ephemeral signing key, in-memory sessions or a published dev
// credential.
func checkProd(opts *auth.ServerOptions) error {
	_, err := runenv.Enforce(func(c *runenv.Checks) {
		c.URL("DB_URL", opts.DBURL)
		c.Require("REDIS_URL", opts.RedisURL)
		c.Require("DUPLI1_AUTH_NATS_URL (or NATS_URL)", opts.NATSURL)
		c.Secret("NATS_TOKEN", natsauth.TokenFromEnv())
		if opts.JWTPrivateKeyFile == "" && len(opts.JWTPrivateKeyPEM) == 0 {
			c.Failf("JWT_PRIVATE_KEY_FILE or JWT_PRIVATE_KEY is required")
		}
		if opts.OwnerEmail != "" {
			c.Secret("OWNER_PASSWORD", opts.OwnerPassword)
		}
		if opts.APIKeyEnv == domain.APIKeyEnvTest {
			c.Failf("DUPLI1_API_KEY_ENV=test mints test keys")
		}
		c.NotDev("DUPLI1_WEB_SERVICE_API_KEY", opts.WebServiceAPIKey)
		c.NotDev("DUPLI1_ORDER_SERVICE_API_KEY", opts.OrderServiceAPIKey)
	})
	return err
}
