package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/elug3/dupli1/auth/pkg/autherrors"
	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

// lastUsedWriteInterval throttles last_used_at writes: a service exchanges
// about every 14 minutes, and the column is for "is this key still in use?",
// not an access log.
const lastUsedWriteInterval = 60 * time.Second

// maxAPIKeyNameLen bounds the operator-chosen label.
const maxAPIKeyNameLen = 100

// WithAPIKeyRepo enables service-account API keys.
func WithAPIKeyRepo(repo ports.APIKeyRepository) ServiceOption {
	return func(s *Service) { s.apiKeyRepo = repo }
}

// WithAccessTokenTTL is the lifetime of an access token, reported as
// expires_in by the key exchange.
func WithAccessTokenTTL(ttl time.Duration) ServiceOption {
	return func(s *Service) { s.accessTokenTTL = ttl }
}

// WithAPIKeyEnv sets the environment marker minted keys carry
// (domain.APIKeyEnvLive or domain.APIKeyEnvTest).
func WithAPIKeyEnv(env string) ServiceOption {
	return func(s *Service) { s.apiKeyEnv = env }
}

// APIKeyExchange is the outcome of ExchangeAPIKey. On a rejection Reason says
// why and Key is set when the key was found; the error is always
// autherrors.ErrInvalidAPIKey, so the caller learns nothing more.
type APIKeyExchange struct {
	AccessToken string
	ExpiresIn   time.Duration
	Key         *domain.APIKey
	Reason      domain.APIKeyRejection
}

// ExchangeAPIKey trades a key for an access token carrying the key's
// effective permissions and the service account's identity. No refresh token
// is issued: the key is the long-lived credential, so a caller that loses its
// access token exchanges again.
func (s *Service) ExchangeAPIKey(ctx context.Context, plaintext string) (APIKeyExchange, error) {
	var out APIKeyExchange
	if s.apiKeyRepo == nil {
		out.Reason = domain.APIKeyUnknown
		return out, autherrors.ErrInvalidAPIKey
	}
	var key *domain.APIKey
	if domain.ValidAPIKeyFormat(plaintext) {
		var err error
		key, err = s.apiKeyRepo.FindByHash(ctx, domain.HashAPIKey(plaintext))
		if err != nil {
			return out, fmt.Errorf("find api key: %w", err)
		}
	}
	out.Key = key
	var owner *domain.User
	if key != nil {
		var err error
		owner, err = s.userRepo.FindByID(ctx, key.UserID)
		if err != nil {
			return out, fmt.Errorf("find api key owner: %w", err)
		}
	}
	now := s.clock()
	if reason := domain.ExchangeRejection(key, owner, now); reason != "" {
		out.Reason = reason
		return out, autherrors.ErrInvalidAPIKey
	}

	token, err := s.tokenGen.Generate(ctx, owner.ID, domain.EffectivePermissions(key, owner), ports.Identity{
		Email:       owner.Email,
		AccountType: domain.AccountTypeService,
		ServiceName: owner.ServiceName,
		APIKeyID:    key.ID,
	})
	if err != nil {
		return out, fmt.Errorf("generate token: %w", err)
	}
	out.AccessToken = token
	out.ExpiresIn = s.accessTokenTTL

	if key.LastUsedAt == nil || now.Sub(*key.LastUsedAt) >= lastUsedWriteInterval {
		// Best effort: a failed bookkeeping write must not fail the caller.
		if err := s.apiKeyRepo.TouchLastUsed(ctx, key.ID, now); err != nil {
			s.logger.Warn().Err(err).Str("event", "api_key_touch_failed").Str("key_id", key.ID).Msg("could not record api key use")
		}
	}
	return out, nil
}

// ServiceAccount returns the service account userID, or ErrUserNotFound /
// ErrNotServiceAccount.
func (s *Service) ServiceAccount(ctx context.Context, userID string) (*domain.User, error) {
	u, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}
	if u == nil {
		return nil, autherrors.ErrUserNotFound
	}
	if domain.NormalizeAccountType(u.AccountType) != domain.AccountTypeService {
		return nil, autherrors.ErrNotServiceAccount
	}
	return u, nil
}

// ListAPIKeys returns the keys of service account userID, revoked included.
func (s *Service) ListAPIKeys(ctx context.Context, userID string) ([]*domain.APIKey, error) {
	if s.apiKeyRepo == nil {
		return nil, errors.New("api keys are not configured")
	}
	if _, err := s.ServiceAccount(ctx, userID); err != nil {
		return nil, err
	}
	return s.apiKeyRepo.ListByUser(ctx, userID)
}

// CreateAPIKey mints a key for service account userID and returns it with its
// plaintext — the only time the plaintext exists outside the caller. An empty
// scope inherits the account's permissions; a scope naming a permission the
// account lacks is refused rather than silently narrowed. ttl 0 never expires.
func (s *Service) CreateAPIKey(ctx context.Context, userID, name string, scope []string, ttl time.Duration, createdBy string) (*domain.APIKey, string, error) {
	if s.apiKeyRepo == nil {
		return nil, "", errors.New("api keys are not configured")
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxAPIKeyNameLen {
		return nil, "", fmt.Errorf("%w: name is required (at most %d characters)", autherrors.ErrInvalidAPIKeyRequest, maxAPIKeyNameLen)
	}
	if ttl < 0 {
		return nil, "", fmt.Errorf("%w: expiry must be positive", autherrors.ErrInvalidAPIKeyRequest)
	}
	scope = permissions.Dedupe(scope)
	if err := permissions.Validate(scope); err != nil {
		return nil, "", fmt.Errorf("%w: %v", autherrors.ErrInvalidPermission, err)
	}
	owner, err := s.ServiceAccount(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	if p := domain.ScopeWithinAccount(scope, owner); p != "" {
		return nil, "", fmt.Errorf("%w: %s", autherrors.ErrScopeExceedsAccount, p)
	}

	plaintext, err := domain.GenerateAPIKey(s.keyEnv())
	if err != nil {
		return nil, "", err
	}
	now := s.clock()
	key := &domain.APIKey{
		ID:          newID(),
		UserID:      owner.ID,
		Name:        name,
		Prefix:      domain.APIKeyPrefix(plaintext),
		KeyHash:     domain.HashAPIKey(plaintext),
		Permissions: scope,
		Source:      domain.APIKeySourceAPI,
		CreatedAt:   now,
		CreatedBy:   createdBy,
	}
	if ttl > 0 {
		exp := now.Add(ttl)
		key.ExpiresAt = &exp
	}
	if err := s.apiKeyRepo.Save(ctx, key); err != nil {
		return nil, "", err
	}
	return key, plaintext, nil
}

// FindAPIKey returns key id or ErrAPIKeyNotFound.
func (s *Service) FindAPIKey(ctx context.Context, id string) (*domain.APIKey, error) {
	if s.apiKeyRepo == nil {
		return nil, autherrors.ErrAPIKeyNotFound
	}
	k, err := s.apiKeyRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if k == nil {
		return nil, autherrors.ErrAPIKeyNotFound
	}
	return k, nil
}

// RevokeAPIKey stops key id from exchanging. Tokens it already minted live
// out their 15 minutes: downstream services validate offline. Revoking twice
// is a no-op. Env-seeded keys are refused (ErrEnvManagedKey): they would come
// back on the next boot.
func (s *Service) RevokeAPIKey(ctx context.Context, id string) (*domain.APIKey, error) {
	k, err := s.FindAPIKey(ctx, id)
	if err != nil {
		return nil, err
	}
	if k.Source == domain.APIKeySourceEnv {
		return k, autherrors.ErrEnvManagedKey
	}
	if k.RevokedAt != nil {
		return k, nil
	}
	now := s.clock()
	k.RevokedAt = &now
	if err := s.apiKeyRepo.Save(ctx, k); err != nil {
		return nil, err
	}
	return k, nil
}

// SyncEnvAPIKey makes the env-seeded key of userID match plaintext, at boot.
// A new value replaces (revokes) the previous env key, so rotating the env
// var rotates the key; an empty value revokes it, so removing the env var
// turns the key off. Keys minted through the API are left alone. It reports
// "" (unchanged), "seeded", "rotated" or "revoked".
func (s *Service) SyncEnvAPIKey(ctx context.Context, userID, plaintext string) (string, error) {
	if s.apiKeyRepo == nil {
		return "", nil
	}
	if plaintext != "" && !domain.ValidAPIKeyFormat(plaintext) {
		return "", errors.New("api key must look like dk_live_<43 base64url chars> (or dk_test_…); see docs/auth-service-api-keys.md → Bootstrap seeding for how to generate one")
	}
	keys, err := s.apiKeyRepo.ListByUser(ctx, userID)
	if err != nil {
		return "", err
	}
	hash := ""
	if plaintext != "" {
		hash = domain.HashAPIKey(plaintext)
	}
	now := s.clock()
	current := false
	revokedAny := false
	for _, k := range keys {
		if k.Source != domain.APIKeySourceEnv {
			continue
		}
		if hash != "" && k.KeyHash == hash {
			if k.RevokedAt != nil {
				k.RevokedAt = nil // the env var was set back to this value
				if err := s.apiKeyRepo.Save(ctx, k); err != nil {
					return "", err
				}
				return "seeded", nil
			}
			current = true
			continue
		}
		if k.RevokedAt == nil {
			k.RevokedAt = &now
			if err := s.apiKeyRepo.Save(ctx, k); err != nil {
				return "", err
			}
			revokedAny = true
		}
	}
	if current || hash == "" {
		if revokedAny {
			return "revoked", nil
		}
		return "", nil
	}
	if other, err := s.apiKeyRepo.FindByHash(ctx, hash); err != nil {
		return "", err
	} else if other != nil {
		return "", errors.New("this api key is already in use by another key; generate a fresh one")
	}
	if err := s.apiKeyRepo.Save(ctx, &domain.APIKey{
		ID:        newID(),
		UserID:    userID,
		Name:      "bootstrap (env)",
		Prefix:    domain.APIKeyPrefix(plaintext),
		KeyHash:   hash,
		Source:    domain.APIKeySourceEnv,
		CreatedAt: now,
	}); err != nil {
		return "", err
	}
	if revokedAny {
		return "rotated", nil
	}
	return "seeded", nil
}

func (s *Service) keyEnv() string {
	if domain.ValidAPIKeyEnv(s.apiKeyEnv) {
		return s.apiKeyEnv
	}
	return domain.APIKeyEnvLive
}

func (s *Service) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}
