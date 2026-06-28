package redis

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// incrWithExpiry atomically increments a key and sets its TTL on the first call.
const incrWithExpiry = `
local n = redis.call('INCR', KEYS[1])
if n == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return n
`

// IPRateLimiter is a Redis-backed per-IP fixed-window rate limiter.
type IPRateLimiter struct {
	client *redis.Client
	prefix string
	max    int
	window int // seconds
	script *redis.Script
}

// NewIPRateLimiter creates a rate limiter allowing max requests per window seconds per IP.
// When client is nil the middleware is a no-op (fail open).
func NewIPRateLimiter(client *redis.Client, prefix string, max int, windowSeconds int) *IPRateLimiter {
	return &IPRateLimiter{
		client: client,
		prefix: prefix,
		max:    max,
		window: windowSeconds,
		script: redis.NewScript(incrWithExpiry),
	}
}

// Middleware returns a Gin handler that enforces the rate limit.
func (l *IPRateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if l.client == nil {
			c.Next()
			return
		}

		key := fmt.Sprintf("rl:%s:%s", l.prefix, c.ClientIP())
		ctx, cancel := context.WithTimeout(c.Request.Context(), 200*time.Millisecond)
		defer cancel()

		n, err := l.script.Run(ctx, l.client, []string{key}, l.window).Int()
		if err != nil {
			// Fail open — don't block requests if Redis is unavailable.
			c.Next()
			return
		}

		if n > l.max {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
			return
		}

		c.Next()
	}
}
