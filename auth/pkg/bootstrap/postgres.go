package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/elug3/dupli1/shared/pkg/pgsslmode"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
)

// withPostgresSSLMode picks a safe sslmode for connURL — see shared/pkg/pgsslmode.
func withPostgresSSLMode(dsn string) string {
	return pgsslmode.WithSSLMode(dsn)
}

func openPostgres(ctx context.Context, connURL string, maxConns int, log zerolog.Logger) (*sql.DB, error) {
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
