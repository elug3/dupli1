package pg

import (
	"errors"
	"fmt"

	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
)

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isFKViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

// wrapDB classifies PostgreSQL errors into ports sentinels while keeping the
// original error in the chain for server logs.
//
// Callers that need a domain-specific sentinel (e.g. ErrMasterInUse on delete)
// should check and return before calling wrapDB.
func wrapDB(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", op, ports.ErrNotFound)
	}
	if isUniqueViolation(err) {
		return fmt.Errorf("%s: %w", op, ports.ErrConflict)
	}
	if isFKViolation(err) {
		return fmt.Errorf("%s: %w", op, ports.ErrInvalid)
	}
	return fmt.Errorf("%s: %w", op, err)
}
