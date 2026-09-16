// Package ratelimit throttles the public promotional-code endpoints.
//
// Redeem and evaluate are unauthenticated and answer "is this a live code" for
// any string, which is a free code-guessing oracle. Before Phase 2 they had no
// limit at all.
//
// Failing open is deliberate. A limiter outage must not stop customers
// checking out, and the limit is not the thing that makes a code safe:
// checkout complete re-evaluates and the ledger enforces one use per customer
// whatever a prober learns. See docs/product-promo-referral-code-plan.md.
package ratelimit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Counter counts hits against a key inside a fixed window.
type Counter interface {
	// Incr returns the count within the current window. An error means the
	// backend is unavailable and the caller lets the request through.
	Incr(ctx context.Context, key string, window time.Duration) (int, error)
}

// Limiter enforces a per-key budget.
type Limiter struct {
	counter Counter
	max     int
	window  time.Duration
}

func New(counter Counter, max int, window time.Duration) *Limiter {
	return &Limiter{counter: counter, max: max, window: window}
}

// Allow reports whether this key still has budget. It is true when the backend
// is missing or erroring, per the fail-open rationale above.
func (l *Limiter) Allow(ctx context.Context, key string) bool {
	if l == nil || l.counter == nil || l.max <= 0 {
		return true
	}
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	n, err := l.counter.Incr(ctx, key, l.window)
	if err != nil {
		return true
	}
	return n <= l.max
}

// Middleware throttles by client IP and, when the request carries one, by
// customer. Both are counted: an IP budget alone is defeated by a botnet, and
// a customer budget alone by staying anonymous.
func (l *Limiter) Middleware(customerOf func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			keys := []string{"ip:" + ClientIP(r)}
			if customerOf != nil {
				if id := customerOf(r); id != "" {
					keys = append(keys, "cust:"+id)
				}
			}
			for _, key := range keys {
				if !l.Allow(r.Context(), key) {
					w.Header().Set("Retry-After", fmt.Sprintf("%d", int(l.window.Seconds())))
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = w.Write([]byte(`{"error":"too many requests","code":429}`))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP prefers the left-most X-Forwarded-For entry, which is the caller as
// seen by the gateway; every request reaches this service through nginx, so
// RemoteAddr alone would bucket the whole world onto the proxy.
func ClientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if first, _, ok := strings.Cut(fwd, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(fwd)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// ── Backends ─────────────────────────────────────────────────────────────────

// incrWithExpiry atomically increments and sets the TTL on the first hit, the
// same script auth's limiter uses.
const incrWithExpiry = `
local n = redis.call('INCR', KEYS[1])
if n == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return n
`

// RedisCounter shares one window across every task, which is what makes the
// configured limit the real limit in production.
type RedisCounter struct {
	client *redis.Client
	prefix string
	script *redis.Script
}

func NewRedisCounter(client *redis.Client, prefix string) *RedisCounter {
	return &RedisCounter{client: client, prefix: prefix, script: redis.NewScript(incrWithExpiry)}
}

func (c *RedisCounter) Incr(ctx context.Context, key string, window time.Duration) (int, error) {
	if c == nil || c.client == nil {
		return 0, fmt.Errorf("redis counter not configured")
	}
	return c.script.Run(ctx, c.client, []string{"rl:" + c.prefix + ":" + key}, int(window.Seconds())).Int()
}

// MemoryCounter keeps the window in this process.
//
// It is the fallback when no Redis is configured, so local dev needs no
// infrastructure. It is per-task: product runs several, so the effective
// budget is the configured one multiplied by the task count. That still caps a
// single prober hammering one task, and the ledger — not this — is what limits
// actual use.
type MemoryCounter struct {
	mu      sync.Mutex
	windows map[string]*memoryWindow
}

type memoryWindow struct {
	count     int
	expiresAt time.Time
}

func NewMemoryCounter() *MemoryCounter {
	return &MemoryCounter{windows: make(map[string]*memoryWindow)}
}

func (c *MemoryCounter) Incr(_ context.Context, key string, window time.Duration) (int, error) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	w, ok := c.windows[key]
	if !ok || now.After(w.expiresAt) {
		c.windows[key] = &memoryWindow{count: 1, expiresAt: now.Add(window)}
		c.sweep(now)
		return 1, nil
	}
	w.count++
	return w.count, nil
}

// sweep drops expired windows so a stream of distinct keys cannot grow the map
// without bound. It runs only when a new window opens, which is cheap.
func (c *MemoryCounter) sweep(now time.Time) {
	if len(c.windows) < 1024 {
		return
	}
	for key, w := range c.windows {
		if now.After(w.expiresAt) {
			delete(c.windows, key)
		}
	}
}
