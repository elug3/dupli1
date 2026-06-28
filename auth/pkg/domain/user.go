package domain

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

// User represents a user entity in the domain.
type User struct {
	ID                   string
	Email                string
	Password             string // hashed
	Roles                []string
	IsActive             bool
	LockedAt             *time.Time
	FailedLoginAttempts  int
}

// NewUser creates a new user, hashing the plaintext password with bcrypt.
func NewUser(id, email, password string, roles ...string) (*User, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return &User{
		ID:       id,
		Email:    email,
		Password: string(hashed),
		Roles:    roles,
		IsActive: true,
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

// HasRole reports whether the user has the given role.
func (u *User) HasRole(role string) bool {
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// ValidatePassword checks the provided plaintext password against the stored bcrypt hash.
func (u *User) ValidatePassword(pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(pw)) == nil
}

// SetRoles replaces the user's role list.
func (u *User) SetRoles(roles []string) {
	u.Roles = roles
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
