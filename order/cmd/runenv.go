package main

import (
	"github.com/elug3/dupli1/shared/pkg/natsauth"
	"github.com/elug3/dupli1/shared/pkg/runenv"
)

// checkProd refuses a production start (DUPLI1_ENV unset or prod) that would
// fall back on an in-memory store, HS256 tokens or a published dev credential.
func checkProd(opts Options) error {
	_, err := runenv.Enforce(func(c *runenv.Checks) {
		c.URL("DUPLI1_ORDER_DB (or DB_URL)", opts.DatabaseConnString)
		c.Require("AUTH_JWKS_URL", opts.JWKSURL)
		c.NotDev("JWT_SECRET", opts.JWTSecret)
		c.Require("DUPLI1_ORDER_NATS_URL (or NATS_URL)", opts.NATSURL)
		c.Secret("NATS_TOKEN", natsauth.TokenFromEnv())
		c.NotDev("DUPLI1_ORDER_SERVICE_API_KEY", opts.OrderServiceAPIKey)
		c.NotDev("DUPLI1_ORDER_S3_SECRET_KEY", opts.S3SecretKey)
	})
	return err
}
