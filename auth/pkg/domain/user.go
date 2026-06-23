package domain

// User represents a user entity in the domain.
type User struct {
	ID       string
	Email    string
	Password string // hashed
	Roles    []string
}

// NewUser creates a new user.
func NewUser(id, email, password string, roles ...string) *User {
	return &User{
		ID:       id,
		Email:    email,
		Password: password,
		Roles:    roles,
	}
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
