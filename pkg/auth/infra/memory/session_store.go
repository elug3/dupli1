package memory

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrSessionNotFound = errors.New("session not found")

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
