package ports

import (
	"context"
	"time"
)

// SessionStore stores refresh-token sessions.
type SessionStore interface {
	Set(ctx context.Context, key string, value string, expiry time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
}
