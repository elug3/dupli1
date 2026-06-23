package domain

import "time"

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

// NewUser creates a new user.
func NewUser(id, email, password string, roles ...string) *User {
	return &User{
		ID:       id,
		Email:    email,
		Password: password,
		Roles:    roles,
		IsActive: true,
	}
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

// ValidatePassword checks the provided password against the stored one.
// NOTE: This is a placeholder — replace with proper hashing comparison.
func (u *User) ValidatePassword(pw string) bool {
	return u.Password == pw
}
