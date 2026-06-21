package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/lib/pq"
)

func openPostgres(ctx context.Context, connURL string, maxConns int) (*sql.DB, error) {
	if connURL == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	connURL = withPostgresSSLMode(connURL)

	db, err := sql.Open("postgres", connURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	if maxConns > 0 {
		db.SetMaxOpenConns(maxConns)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return db, nil
}

// withPostgresSSLMode sets sslmode when it is not already present.
// Local/docker hosts default to disable; managed databases default to require.
func withPostgresSSLMode(connURL string) string {
	if strings.Contains(connURL, "sslmode=") {
		return connURL
	}

	mode := "require"
	if isLocalPostgresHost(connURL) {
		mode = "disable"
	}

	return setSSLMode(connURL, mode)
}

func isLocalPostgresHost(connURL string) bool {
	host := postgresHost(connURL)
	if host == "" {
		return false
	}

	switch host {
	case "localhost", "127.0.0.1", "postgres-auth", "postgres-product", "postgres":
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
