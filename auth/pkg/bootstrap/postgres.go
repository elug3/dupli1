package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
)

// withPostgresSSLMode appends or preserves an sslmode parameter.
// URL-format DSNs get sslmode=require unless the host ends in ".local".
// Key-value DSNs get sslmode=disable.
// Existing sslmode values are never overwritten.
func withPostgresSSLMode(dsn string) string {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		// URL format
		if strings.Contains(dsn, "sslmode=") {
			return dsn
		}
		u, err := url.Parse(dsn)
		if err != nil {
			return dsn
		}
		host := u.Hostname()
		mode := "require"
		if strings.HasSuffix(host, ".local") || host == "localhost" || host == "127.0.0.1" {
			mode = "disable"
		}
		sep := "?"
		if u.RawQuery != "" {
			sep = "&"
		}
		return dsn + sep + "sslmode=" + mode
	}
	// Key-value DSN format
	if strings.Contains(dsn, "sslmode=") {
		return dsn
	}
	return dsn + " sslmode=disable"
}

func openPostgres(ctx context.Context, connURL string, maxConns int, log zerolog.Logger) (*sql.DB, error) {
	if connURL == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	db, err := sql.Open("postgres", connURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	if maxConns > 0 {
		db.SetMaxOpenConns(maxConns)
	}

	log.Info().Msg("Connecting to database...")

	deadline := time.Now().Add(60 * time.Second)
	hasDots := false

	for {
		pingCtx, cancel := context.WithTimeout(ctx, time.Second)
		pingErr := db.PingContext(pingCtx)
		cancel()

		if pingErr == nil {
			if hasDots {
				fmt.Println()
			}
			log.Info().Msg("connected")
			return db, nil
		}

		if ctx.Err() != nil || time.Now().After(deadline) {
			if hasDots {
				fmt.Println()
			}
			log.Error().Msg("connection timeout")
			_ = db.Close()
			return nil, errors.New("connection timeout")
		}

		fmt.Print(".")
		hasDots = true

		select {
		case <-ctx.Done():
			fmt.Println()
			log.Error().Msg("connection timeout")
			_ = db.Close()
			return nil, errors.New("connection timeout")
		case <-time.After(time.Second):
		}
	}
}

