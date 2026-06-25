package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// SessionCache provides session caching using Redis.
type SessionCache struct {
	client *redis.Client
}

// NewSessionCache creates a new Redis session cache.
func NewSessionCache(client *redis.Client) *SessionCache {
	return &SessionCache{client: client}
}

// Set stores a session.
func (sc *SessionCache) Set(ctx context.Context, key string, value string, expiry time.Duration) error {
	return sc.client.Set(ctx, key, value, expiry).Err()
}

// Get retrieves a session.
func (sc *SessionCache) Get(ctx context.Context, key string) (string, error) {
	return sc.client.Get(ctx, key).Result()
}

// Delete removes a session.
func (sc *SessionCache) Delete(ctx context.Context, key string) error {
	return sc.client.Del(ctx, key).Err()
}
