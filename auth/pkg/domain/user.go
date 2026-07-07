package domain

import (
	"time"

	"github.com/elug3/dupli1/shared/pkg/permissions"
	"golang.org/x/crypto/bcrypt"
)

// User represents a user entity in the domain.
type User struct {
	ID                  string
	Email               string
	Password            string // hashed
	AccountType         string
	Permissions         []string
	IsActive            bool
	LockedAt            *time.Time
	FailedLoginAttempts int
}

// NewUser creates a new user, hashing the plaintext password with bcrypt.
func NewUser(id, email, password, accountType string, perms ...string) (*User, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return &User{
		ID:          id,
		Email:       email,
		Password:    string(hashed),
		AccountType: accountType,
		Permissions: permissions.Dedupe(perms),
		IsActive:    true,
	}, nil
}

// IsLocked reports whether the account is currently locked.
func (u *User) IsLocked() bool {
	return u.LockedAt != nil
}

// Lock sets LockedAt to now.
func (u *User) Lock() {
	now := time.Now()
	u.LockedAt = &now
}

// Unlock clears the lock and resets failed attempts.
func (u *User) Unlock() {
	u.LockedAt = nil
	u.FailedLoginAttempts = 0
}

// HasPermission reports whether the user holds any of the given permissions.
func (u *User) HasPermission(required ...string) bool {
	return permissions.HasAny(u.Permissions, required...)
}

// ValidatePassword checks the provided plaintext password against the stored bcrypt hash.
func (u *User) ValidatePassword(pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(pw)) == nil
}

// SetPermissions replaces the user's permission list.
func (u *User) SetPermissions(perms []string) {
	u.Permissions = permissions.Dedupe(perms)
}

// UpdatePassword hashes plaintext and replaces the stored password.
func (u *User) UpdatePassword(plaintext string) error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Password = string(hashed)
	return nil
}

// SetActive sets the user's active status.
func (u *User) SetActive(active bool) {
	u.IsActive = active
}
