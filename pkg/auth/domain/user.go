package domain

// User represents a user entity in the domain.
type User struct {
	ID       string
	Email    string
	Password string // hashed
}

// NewUser creates a new user.
func NewUser(id, email, password string) *User {
	return &User{
		ID:       id,
		Email:    email,
		Password: password,
	}
}

// ValidatePassword checks the provided password against the stored one.
// NOTE: This is a placeholder — replace with proper hashing comparison.
func (u *User) ValidatePassword(pw string) bool {
	return u.Password == pw
}
