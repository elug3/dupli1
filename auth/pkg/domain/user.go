package domain

import (
	"strings"
	"time"

	"github.com/elug3/dupli1/shared/pkg/permissions"
	"golang.org/x/crypto/bcrypt"
)

// User represents a user entity in the domain.
type User struct {
	ID          string
	Email       string
	Password    string // hashed
	AccountType string
	// ServiceName names a service account (e.g. dupli1-order); empty for
	// people. Internal APIs check it, so only the startup seeds set it.
	ServiceName         string
	Permissions         []string
	IsActive            bool
	LockedAt            *time.Time
	FailedLoginAttempts int
}

// NormalizeEmail trims whitespace and lowercases an email address so
// "Alice@x.com" and "alice@x.com" are always treated as the same account.
// Every path that sets User.Email (creation, admin reset, lookups) should
// route through this so the DB's case-insensitive unique index and the
// application layer agree on one canonical form.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// NewUser creates a new user, hashing the plaintext password with bcrypt.
func NewUser(id, email, password, accountType string, perms ...string) (*User, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return &User{
		ID:          id,
		Email:       NormalizeEmail(email),
		Password:    string(hashed),
		AccountType: accountType,
		Permissions: permissions.Dedupe(perms),
		IsActive:    true,
	}, nil
}

// NewPasswordlessUser creates an account with no password at all — a service
// account, which authenticates only with API keys. Nothing is hashed, so no
// password ever existed to leak or reset.
func NewPasswordlessUser(id, email, accountType string, perms ...string) *User {
	return &User{
		ID:          id,
		Email:       NormalizeEmail(email),
		Password:    retiredPasswordHash,
		AccountType: accountType,
		Permissions: permissions.Dedupe(perms),
		IsActive:    true,
	}
}

// AccountLockDuration is how long a failed-login lockout lasts before it
// automatically lifts. There is no unlock endpoint, so a permanent lock would
// leave a customer's account (or one an attacker deliberately locks, knowing
// only the email address) unrecoverable through the API.
const AccountLockDuration = 15 * time.Minute

// IsLocked reports whether the account is currently locked.
// Admin and owner accounts are never considered locked (see IsLockExempt).
// A lock automatically expires after AccountLockDuration.
func (u *User) IsLocked() bool {
	if u == nil || u.IsLockExempt() {
		return false
	}
	return u.LockedAt != nil && time.Since(*u.LockedAt) < AccountLockDuration
}

// IsLockExempt reports whether failed-login lockout must not apply.
// Owners (`*` permission) and admin-tier operators (account_type manager with
// admin-level permissions such as admin.*) cannot be locked out of manage-web.
func (u *User) IsLockExempt() bool {
	if u == nil {
		return false
	}
	switch UserClass(u) {
	case ClassOwner:
		return true
	case ClassAdmin:
		return NormalizeAccountType(u.AccountType) == AccountTypeManager
	default:
		return false
	}
}

// Lock sets LockedAt to now. No-op for lock-exempt admin/owner accounts.
func (u *User) Lock() {
	if u == nil || u.IsLockExempt() {
		return
	}
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
	if u.PasswordRetired() {
		// Same cost as a real comparison, so timing doesn't reveal which
		// accounts have no password.
		ValidateDummyPassword(pw)
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(pw)) == nil
}

// retiredPasswordHash is stored instead of a bcrypt hash for an account that
// has no password — a service account that authenticates with an API key
// only. No input matches it.
const retiredPasswordHash = "!"

// RetirePassword removes password login from the account.
func (u *User) RetirePassword() { u.Password = retiredPasswordHash }

// PasswordRetired reports whether the account has no password.
func (u *User) PasswordRetired() bool { return u.Password == retiredPasswordHash }

// dummyPasswordHash is a bcrypt hash (at bcrypt.DefaultCost, same as every
// real account) of a placeholder password nobody's account uses.
const dummyPasswordHash = "$2a$10$.R70sTG1DFOsiLclagRoHOsSzV5rBKgnzOLALTGxxTA5QSqJ2v5HW"

// ValidateDummyPassword runs a bcrypt comparison against a fixed placeholder
// hash nothing can match. Login calls this on its "no such account" path so
// that path costs about the same as ValidatePassword against a real account
// — otherwise the response-time gap tells an unauthenticated caller whether
// an email is registered.
func ValidateDummyPassword(pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(dummyPasswordHash), []byte(pw)) == nil
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
