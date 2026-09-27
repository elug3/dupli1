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
)

// ClientNotAllowedError is a login refused because the account type does not
// belong to the requesting client. Reason is written for the person signing
// in; it matches ErrClientNotAllowed under errors.Is.
type ClientNotAllowedError struct {
	Reason string
}

func (e *ClientNotAllowedError) Error() string { return e.Reason }

func (e *ClientNotAllowedError) Is(target error) bool { return target == ErrClientNotAllowed }
