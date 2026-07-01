package memory

import (
	"context"
	"sync"
	"time"

	"github.com/elug3/dupli1/auth/pkg/ports"
)

// ErrSessionNotFound is kept for backwards compatibility; prefer ports.ErrSessionNotFound.
var ErrSessionNotFound = ports.ErrSessionNotFound

type sessionEntry struct {
	value     string
	expiresAt time.Time
}

// SessionStore stores refresh sessions in memory.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]sessionEntry
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]sessionEntry),
	}
}

// GC starts a background goroutine that evicts expired entries at the given interval.
// The goroutine stops when ctx is cancelled.
func (s *SessionStore) GC(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.evictExpired()
			}
		}
	}()
}

func (s *SessionStore) evictExpired() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, entry := range s.sessions {
		if now.After(entry.expiresAt) {
			delete(s.sessions, key)
		}
	}
}

func (s *SessionStore) Set(ctx context.Context, key string, value string, expiry time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[key] = sessionEntry{
		value:     value,
		expiresAt: time.Now().Add(expiry),
	}
	return nil
}

func (s *SessionStore) Get(ctx context.Context, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	s.mu.RLock()
	entry, ok := s.sessions[key]
	s.mu.RUnlock()
	if !ok {
		return "", ErrSessionNotFound
	}
	if time.Now().After(entry.expiresAt) {
		_ = s.Delete(ctx, key)
		return "", ErrSessionNotFound
	}

	return entry.value, nil
}

func (s *SessionStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, key)
	return nil
}
