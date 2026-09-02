// Package pgsslmode picks a safe sslmode for a Postgres connection string.
// Known local/docker-compose hostnames (and any host ending in .local)
// default to "disable"; every other host — including RDS in production —
// defaults to "require". An sslmode already present in the connection
// string is always left untouched.
//
// Every service previously carried its own copy of this logic, and the
// copies had drifted: each service's local-hostname list only knew about
// whichever docker-compose Postgres containers existed when that copy was
// last touched, and two services (auth, notification) had independently
// written different, less-safe algorithms — notification defaulted every
// unknown host to "disable" instead of "require", which is the wrong
// default for a database URL that omits sslmode and turns out to be RDS.
package pgsslmode

import (
	"net/url"
	"strings"
)

// localHosts are the docker-compose Postgres service names, plus loopback
// hosts, that run without TLS in local development. Keep in sync with the
// postgres-* service names in docker-compose.yml.
var localHosts = map[string]bool{
	"localhost":             true,
	"127.0.0.1":             true,
	"postgres":              true,
	"postgres-auth":         true,
	"postgres-product":      true,
	"postgres-order":        true,
	"postgres-cart":         true,
	"postgres-payment":      true,
	"postgres-notification": true,
}

// WithSSLMode appends an sslmode parameter when connURL doesn't already
// specify one, for both postgres:// URL and key=value DSN formats. Known
// local/docker hosts (and any host ending in .local) get "disable"; every
// other host — including managed databases like RDS — gets "require".
func WithSSLMode(connURL string) string {
	if strings.Contains(connURL, "sslmode=") {
		return connURL
	}
	mode := "require"
	if isLocalHost(connURL) {
		mode = "disable"
	}
	return setSSLMode(connURL, mode)
}

func isLocalHost(connURL string) bool {
	host := postgresHost(connURL)
	if host == "" {
		return false
	}
	if localHosts[host] {
		return true
	}
	return strings.HasSuffix(host, ".local")
}

func postgresHost(connURL string) string {
	if strings.HasPrefix(connURL, "postgres://") || strings.HasPrefix(connURL, "postgresql://") {
		parsed, err := url.Parse(connURL)
		if err != nil {
			return ""
		}
		return parsed.Hostname()
	}

	for _, field := range strings.Fields(connURL) {
		if strings.HasPrefix(field, "host=") {
			return strings.TrimPrefix(field, "host=")
		}
	}
	return ""
}

func setSSLMode(connURL, mode string) string {
	if strings.HasPrefix(connURL, "postgres://") || strings.HasPrefix(connURL, "postgresql://") {
		parsed, err := url.Parse(connURL)
		if err != nil {
			sep := "?"
			if strings.Contains(connURL, "?") {
				sep = "&"
			}
			return connURL + sep + "sslmode=" + mode
		}

		query := parsed.Query()
		query.Set("sslmode", mode)
		parsed.RawQuery = query.Encode()
		return parsed.String()
	}

	if !strings.Contains(connURL, " ") {
		return connURL + " sslmode=" + mode
	}
	return strings.TrimSpace(connURL) + " sslmode=" + mode
}
