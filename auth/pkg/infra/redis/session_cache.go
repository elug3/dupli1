package redis

import (
	"context"
	"errors"
	"time"

	"github.com/elug3/dupli1/auth/pkg/ports"
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

// Get retrieves a session. Returns ports.ErrSessionNotFound when the key is absent or expired.
func (sc *SessionCache) Get(ctx context.Context, key string) (string, error) {
	val, err := sc.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ports.ErrSessionNotFound
	}
	return val, err
}

// Delete removes a session.
func (sc *SessionCache) Delete(ctx context.Context, key string) error {
	return sc.client.Del(ctx, key).Err()
}

var rotateSessionScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
  return 0
end
redis.call('SET', KEYS[2], ARGV[1], 'EX', ARGV[2])
redis.call('DEL', KEYS[1])
return 1
`)

// Rotate atomically replaces oldKey with newKey.
func (sc *SessionCache) Rotate(ctx context.Context, oldKey, newKey, value string, expiry time.Duration) error {
	ttlSec := int64(expiry / time.Second)
	if ttlSec < 1 {
		ttlSec = 1
	}
	n, err := rotateSessionScript.Run(ctx, sc.client, []string{oldKey, newKey}, value, ttlSec).Int64()
	if err != nil {
		return err
	}
	if n == 0 {
		return ports.ErrSessionNotFound
	}
	return nil
}
