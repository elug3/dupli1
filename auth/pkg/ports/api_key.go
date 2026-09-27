package ports

import (
	"context"
	"time"

	"github.com/elug3/dupli1/auth/pkg/domain"
)

// APIKeyRepository persists service-account API keys. Lookups return
// (nil, nil) when nothing matches.
type APIKeyRepository interface {
	// FindByHash returns the key whose SHA-256 is hash.
	FindByHash(ctx context.Context, hash string) (*domain.APIKey, error)
	FindByID(ctx context.Context, id string) (*domain.APIKey, error)
	// ListByUser returns every key of userID, revoked ones included, newest first.
	ListByUser(ctx context.Context, userID string) ([]*domain.APIKey, error)
	// Save creates or updates a key.
	Save(ctx context.Context, key *domain.APIKey) error
	// TouchLastUsed records a successful exchange.
	TouchLastUsed(ctx context.Context, id string, at time.Time) error
}
