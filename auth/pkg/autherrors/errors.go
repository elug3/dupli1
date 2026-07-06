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
)
