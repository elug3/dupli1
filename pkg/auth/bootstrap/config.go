package bootstrap

import "time"

// Config holds the dependencies required to wire the auth service.
type Config struct {
	DBURL              string
	RedisURL           string
	NATSURL            string
	TokenSigningKey    []byte
	TokenExpiry        time.Duration
	RefreshTokenExpiry time.Duration
	Debug              bool
	MaxConns           int

	// OwnerEmail and OwnerPassword seed an owner user on first startup.
	// When OwnerEmail is empty, seeding is skipped.
	OwnerEmail    string
	OwnerPassword string
}
