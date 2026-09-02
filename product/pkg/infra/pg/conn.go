package pg

import "github.com/elug3/dupli1/shared/pkg/pgsslmode"

// withPostgresSSLMode picks a safe sslmode for connURL — see shared/pkg/pgsslmode.
func withPostgresSSLMode(connURL string) string {
	return pgsslmode.WithSSLMode(connURL)
}
