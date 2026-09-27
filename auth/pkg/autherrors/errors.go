package autherrors

import "errors"

// Common errors for the auth service.
var (
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrUserNotFound        = errors.New("user not found")
	ErrUserAlreadyExists   = errors.New("user already exists")
	ErrInvalidToken        = errors.New("invalid token")
	ErrTokenExpired        = errors.New("token expired")
	ErrInvalidEmail        = errors.New("invalid email")
	ErrWeakPassword        = errors.New("password is too weak")
	ErrAccountLocked       = errors.New("account is locked")
	ErrAccountDeactivated  = errors.New("account is deactivated")
	ErrInvalidAccountType  = errors.New("invalid account type")
	ErrInvalidPermission   = errors.New("invalid permission")
	ErrManagementForbidden = errors.New("management forbidden")
	ErrInvalidClient       = errors.New("invalid client")
	ErrClientNotAllowed    = errors.New("account type not allowed for this client")
	// ErrInvalidAPIKey covers every failed key exchange — unknown, revoked,
	// expired, inactive or non-service account — so the caller cannot tell
	// which. The reason is logged.
	ErrInvalidAPIKey  = errors.New("invalid api key")
	ErrAPIKeyNotFound = errors.New("api key not found")
	// ErrInvalidAPIKeyRequest: a create request with a bad name or expiry.
	ErrInvalidAPIKeyRequest = errors.New("invalid api key request")
	// ErrEnvManagedKey refuses to revoke a key seeded from an env var: it
	// would reappear on the next auth boot. Rotate the env var instead.
	ErrEnvManagedKey = errors.New("api key is managed by an environment variable")
	// ErrNotServiceAccount: API keys attach to service accounts only.
	ErrNotServiceAccount = errors.New("api keys are for service accounts only")
	// ErrScopeExceedsAccount: a key's scope names a permission its account lacks.
	ErrScopeExceedsAccount = errors.New("api key scope exceeds the account's permissions")
	// ErrServiceAccountPassword: service accounts have no password — they
	// authenticate with API keys — so one can be neither set at registration
	// nor reset later.
	ErrServiceAccountPassword = errors.New("service accounts do not use passwords; use api keys")
)

// ClientNotAllowedError is a login refused because the account type does not
// belong to the requesting client. Reason is written for the person signing
// in; it matches ErrClientNotAllowed under errors.Is.
type ClientNotAllowedError struct {
	Reason string
}

func (e *ClientNotAllowedError) Error() string { return e.Reason }

func (e *ClientNotAllowedError) Is(target error) bool { return target == ErrClientNotAllowed }
