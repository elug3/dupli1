package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"time"

	"github.com/elug3/dupli1/shared/pkg/permissions"
)

// An APIKey is a long-lived credential for a service account. It is exchanged
// at POST /api/v1/auth/token for an ordinary access token, so services that
// validate tokens need no change. Only its SHA-256 is stored: the plaintext is
// shown once, when minted, and cannot be recovered. See
// docs/auth-service-api-keys.md.
type APIKey struct {
	ID     string
	UserID string
	Name   string
	// Prefix is the first APIKeyPrefixLen characters of the plaintext
	// ("dk_live_A1b2"), for display and logs. Never used to authenticate.
	Prefix  string
	KeyHash string
	// Permissions scopes the key. Empty means the account's own permissions.
	Permissions []string
	// Source is APIKeySourceAPI (minted by an operator) or APIKeySourceEnv
	// (seeded from a bootstrap env var and re-synced on every auth boot).
	Source    string
	CreatedAt time.Time
	CreatedBy string
	ExpiresAt *time.Time
	// LastUsedAt is written only by the repository's TouchLastUsed.
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

const (
	APIKeySourceAPI = "api"
	APIKeySourceEnv = "env"

	// APIKeyEnvLive and APIKeyEnvTest are the environment markers in a key's
	// prefix, so a leaked key says where it works and is easy to grep for.
	APIKeyEnvLive = "live"
	APIKeyEnvTest = "test"

	// APIKeyPrefixLen is how much of the plaintext is kept for display.
	APIKeyPrefixLen = 12

	// apiKeyRandomBytes is the key's entropy: 256 bits from crypto/rand.
	apiKeyRandomBytes = 32
)

// apiKeyFormat is dk_<env>_ then 32 bytes as unpadded base64url (43 chars).
var apiKeyFormat = regexp.MustCompile(`^dk_(live|test)_[A-Za-z0-9_-]{43}$`)

// ValidAPIKeyEnv reports whether env is a known key environment marker.
func ValidAPIKeyEnv(env string) bool {
	return env == APIKeyEnvLive || env == APIKeyEnvTest
}

// GenerateAPIKey returns a new plaintext key for env ("live" or "test").
func GenerateAPIKey(env string) (string, error) {
	if !ValidAPIKeyEnv(env) {
		return "", fmt.Errorf("unknown api key environment %q", env)
	}
	buf := make([]byte, apiKeyRandomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return "dk_" + env + "_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// ValidAPIKeyFormat reports whether plaintext has the shape GenerateAPIKey
// produces. Seeded env keys must pass it, so a weak hand-typed value cannot
// become a service credential.
func ValidAPIKeyFormat(plaintext string) bool {
	return apiKeyFormat.MatchString(plaintext)
}

// HashAPIKey is the stored form of a key. SHA-256 with no salt is right here,
// unlike for passwords: the input is 256 uniformly random bits, so there is
// nothing for a slow hash to protect, and an unsalted digest can be looked up
// by index in one step.
func HashAPIKey(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// APIKeyPrefix returns the displayable start of plaintext.
func APIKeyPrefix(plaintext string) string {
	if len(plaintext) <= APIKeyPrefixLen {
		return plaintext
	}
	return plaintext[:APIKeyPrefixLen]
}

// APIKeyRejection says why a key did not exchange. The caller always gets the
// same 401; the reason is for the operator's log.
type APIKeyRejection string

const (
	APIKeyUnknown    APIKeyRejection = "unknown"
	APIKeyRevoked    APIKeyRejection = "revoked"
	APIKeyExpired    APIKeyRejection = "expired"
	APIKeyInactive   APIKeyRejection = "inactive"
	APIKeyNotService APIKeyRejection = "not_service"
)

// ExchangeRejection returns why key (belonging to owner) may not be exchanged
// at now, or "" when it may. Account lockout is deliberately not checked: it
// defends passwords, and letting it gate keys would let anyone who guesses a
// service account's email lock it with bad passwords and take its caller down.
// is_active stays the account-wide switch; revoking the key is the targeted one.
func ExchangeRejection(key *APIKey, owner *User, now time.Time) APIKeyRejection {
	switch {
	case key == nil:
		return APIKeyUnknown
	case key.RevokedAt != nil:
		return APIKeyRevoked
	case key.ExpiresAt != nil && !now.Before(*key.ExpiresAt):
		return APIKeyExpired
	case owner == nil || !owner.IsActive:
		return APIKeyInactive
	case NormalizeAccountType(owner.AccountType) != AccountTypeService:
		return APIKeyNotService
	}
	return ""
}

// EffectivePermissions is what a token minted from key carries: the key's
// scope intersected with the account's current permissions, recomputed at
// every exchange so narrowing the account narrows its keys at once. An empty
// scope inherits the account's permissions.
func EffectivePermissions(key *APIKey, owner *User) []string {
	if len(key.Permissions) == 0 {
		return permissions.Dedupe(owner.Permissions)
	}
	out := make([]string, 0, len(key.Permissions))
	for _, p := range key.Permissions {
		if permissions.Has(owner.Permissions, p) {
			out = append(out, p)
		}
	}
	return permissions.Dedupe(out)
}

// ScopeWithinAccount returns the first requested permission the account does
// not hold, or "" when the whole scope fits. A key can never exceed its account.
func ScopeWithinAccount(scope []string, owner *User) string {
	for _, p := range scope {
		if !permissions.Has(owner.Permissions, p) {
			return p
		}
	}
	return ""
}
