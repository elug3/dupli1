package bootstrap

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

func openPostgres(ctx context.Context, url string, maxConns int) (*sql.DB, error) {
	if url == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	db, err := sql.Open("postgres", url)
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
