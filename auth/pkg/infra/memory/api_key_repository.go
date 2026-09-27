package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/elug3/dupli1/auth/pkg/domain"
)

// APIKeyRepository is an in-memory ports.APIKeyRepository for tests.
type APIKeyRepository struct {
	mu   sync.Mutex
	keys map[string]domain.APIKey
}

// NewAPIKeyRepository creates an empty in-memory API key repository.
func NewAPIKeyRepository() *APIKeyRepository {
	return &APIKeyRepository{keys: make(map[string]domain.APIKey)}
}

func (r *APIKeyRepository) FindByHash(_ context.Context, hash string) (*domain.APIKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, k := range r.keys {
		if k.KeyHash == hash {
			return copyKey(k), nil
		}
	}
	return nil, nil
}

func (r *APIKeyRepository) FindByID(_ context.Context, id string) (*domain.APIKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k, ok := r.keys[id]
	if !ok {
		return nil, nil
	}
	return copyKey(k), nil
}

func (r *APIKeyRepository) ListByUser(_ context.Context, userID string) ([]*domain.APIKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.APIKey
	for _, k := range r.keys {
		if k.UserID == userID {
			out = append(out, copyKey(k))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Save creates or updates a key; like the Postgres store, an update keeps
// the stored LastUsedAt (TouchLastUsed owns it).
func (r *APIKeyRepository) Save(_ context.Context, k *domain.APIKey) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := *copyKey(*k)
	if prev, ok := r.keys[k.ID]; ok {
		next.LastUsedAt = prev.LastUsedAt
	}
	r.keys[k.ID] = next
	return nil
}

func (r *APIKeyRepository) TouchLastUsed(_ context.Context, id string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if k, ok := r.keys[id]; ok {
		k.LastUsedAt = &at
		r.keys[id] = k
	}
	return nil
}

func copyKey(k domain.APIKey) *domain.APIKey {
	k.Permissions = append([]string(nil), k.Permissions...)
	return &k
}
