package ports

import (
	"errors"
	"fmt"
)

// Sentinel errors for the product service boundary.
// Infra wraps driver/SQL failures with these so handlers can map status codes
// without inspecting pgx/pgconn types or leaking raw database messages.
var (
	// ErrNotFound is a missing product, variant, coupon, or other resource.
	ErrNotFound = errors.New("not found")
	// ErrConflict is a uniqueness or in-use conflict (duplicate key, etc.).
	ErrConflict = errors.New("conflict")
	// ErrInvalid is a client-correctable validation / input error.
	ErrInvalid = errors.New("invalid request")
)

// Invalid wraps a human-readable validation message with ErrInvalid.
func Invalid(msg string) error {
	return fmt.Errorf("%w: %s", ErrInvalid, msg)
}

// NotFound wraps a resource-specific message with ErrNotFound.
func NotFound(msg string) error {
	return fmt.Errorf("%w: %s", ErrNotFound, msg)
}

// Conflict wraps a conflict message with ErrConflict.
func Conflict(msg string) error {
	return fmt.Errorf("%w: %s", ErrConflict, msg)
}
