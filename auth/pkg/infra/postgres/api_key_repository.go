package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/lib/pq"
)

// APIKeyRepository implements ports.APIKeyRepository using PostgreSQL.
type APIKeyRepository struct {
	db *sql.DB
}

// NewAPIKeyRepository creates a PostgreSQL API key repository.
func NewAPIKeyRepository(db *sql.DB) *APIKeyRepository {
	return &APIKeyRepository{db: db}
}

const apiKeyColumns = `id, user_id, name, prefix, key_hash, permissions, source,
	created_at, created_by, expires_at, last_used_at, revoked_at`

// FindByHash is one indexed lookup on ux_service_api_keys_hash.
func (r *APIKeyRepository) FindByHash(ctx context.Context, hash string) (*domain.APIKey, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+apiKeyColumns+` FROM service_api_keys WHERE key_hash = $1`, hash)
	return scanAPIKey(row)
}

// FindByID returns the key with id.
func (r *APIKeyRepository) FindByID(ctx context.Context, id string) (*domain.APIKey, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+apiKeyColumns+` FROM service_api_keys WHERE id = $1`, id)
	return scanAPIKey(row)
}

// ListByUser returns every key of userID, newest first.
func (r *APIKeyRepository) ListByUser(ctx context.Context, userID string) ([]*domain.APIKey, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+apiKeyColumns+` FROM service_api_keys WHERE user_id = $1 ORDER BY created_at DESC, id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()
	var keys []*domain.APIKey
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, fmt.Errorf("list api keys: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// Save creates or updates a key. An update leaves last_used_at alone:
// TouchLastUsed is its only writer, so a revoke that loaded the row before an
// exchange cannot erase that exchange.
func (r *APIKeyRepository) Save(ctx context.Context, k *domain.APIKey) error {
	perms := k.Permissions
	if perms == nil {
		perms = []string{}
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO service_api_keys (id, user_id, name, prefix, key_hash, permissions, source,
			created_at, created_by, expires_at, last_used_at, revoked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			name = $3, prefix = $4, key_hash = $5, permissions = $6, source = $7,
			expires_at = $10, revoked_at = $12`,
		k.ID, k.UserID, k.Name, k.Prefix, k.KeyHash, pq.Array(perms), k.Source,
		k.CreatedAt, k.CreatedBy, k.ExpiresAt, k.LastUsedAt, k.RevokedAt)
	if err != nil {
		return fmt.Errorf("save api key: %w", err)
	}
	return nil
}

// TouchLastUsed records a successful exchange.
func (r *APIKeyRepository) TouchLastUsed(ctx context.Context, id string, at time.Time) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE service_api_keys SET last_used_at = $2 WHERE id = $1`, id, at); err != nil {
		return fmt.Errorf("touch api key: %w", err)
	}
	return nil
}

func scanAPIKey(s scanner) (*domain.APIKey, error) {
	var k domain.APIKey
	var expiresAt, lastUsedAt, revokedAt sql.NullTime
	err := s.Scan(&k.ID, &k.UserID, &k.Name, &k.Prefix, &k.KeyHash, pq.Array(&k.Permissions), &k.Source,
		&k.CreatedAt, &k.CreatedBy, &expiresAt, &lastUsedAt, &revokedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan api key: %w", err)
	}
	k.ExpiresAt = nullTime(expiresAt)
	k.LastUsedAt = nullTime(lastUsedAt)
	k.RevokedAt = nullTime(revokedAt)
	return &k, nil
}

func nullTime(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
