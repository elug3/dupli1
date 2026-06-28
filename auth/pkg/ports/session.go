package ports

import (
	"context"
	"errors"
	"time"
)

// ErrSessionNotFound is returned by SessionStore.Get when the key does not exist or has expired.
var ErrSessionNotFound = errors.New("session not found")

// SessionStore stores refresh-token sessions.
type SessionStore interface {
	Set(ctx context.Context, key string, value string, expiry time.Duration) error
	// Get returns ErrSessionNotFound when the key does not exist or has expired.
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
}
