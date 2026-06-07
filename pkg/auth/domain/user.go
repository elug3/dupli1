package domain

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
)

// User represents a user entity in the domain.
type User struct {
	ID       uuid.UUID
	Email    string
	Password string // argon2id encoded hash
}

// NewUser creates a new user with the provided id and data.
func NewUser(email string) *User {

	id := uuid.New()

	return &User{
		ID:    id,
		Email: email,
	}
}

// SetPassword hashes the provided plaintext password and stores it.
func (u *User) SetPassword(plain string) error {
	hash, err := hashPassword(plain)
	if err != nil {
		return err
	}
	u.Password = hash
	return nil
}

// ValidatePassword verifies the provided plaintext password against stored hash.
func (u *User) ValidatePassword(plain string) bool {
	ok, _ := comparePassword(u.Password, plain)
	return ok
}

// Argon2id parameters
const (
	argonTime    uint32 = 1
	argonMemory  uint32 = 64 * 1024
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32
	saltLen             = 16
)

// hashPassword returns an encoded argon2id hash for storage.
func hashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	encoded := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads, b64Salt, b64Hash)
	return encoded, nil
}

// comparePassword compares encoded argon2id hash with plaintext password.
func comparePassword(encoded, password string) (bool, error) {
	// encoded format: $argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return false, fmt.Errorf("invalid encoded hash format")
	}

	// parts[3] contains params like m=65536,t=1,p=4
	params := parts[3]
	var memory uint32
	var timeParam uint32
	var threads uint8
	_, err := fmt.Sscanf(params, "m=%d,t=%d,p=%d", &memory, &timeParam, &threads)
	if err != nil {
		return false, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}

	computed := argon2.IDKey([]byte(password), salt, timeParam, memory, threads, uint32(len(hash)))

	if subtleCompare(computed, hash) {
		return true, nil
	}
	return false, nil
}

// subtleCompare does constant-time comparison of two byte slices.
func subtleCompare(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
