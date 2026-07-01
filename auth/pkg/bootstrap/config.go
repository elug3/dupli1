package bootstrap

import (
	"time"

	"github.com/rs/zerolog"
)

// Config holds the dependencies required to wire the auth service.
type Config struct {
	DBURL              string
	RedisURL           string
	NATSURL            string
	JWTPrivateKeyPEM   []byte   // PEM-encoded RSA private key for RS256 / JWKS mode
	JWTKeyID           string   // "kid" value in the JWKS document; defaults to "default"
	TokenExpiry        time.Duration
	RefreshTokenExpiry time.Duration
	CORSOrigins        []string // allowed CORS origins; empty = no CORS headers
	Debug              bool
	MaxConns           int
	Logger             zerolog.Logger

	// OwnerEmail and OwnerPassword seed an owner user on first startup.
	// When OwnerEmail is empty, seeding is skipped.
	OwnerEmail    string
	OwnerPassword string

	// WebServiceEmail and WebServicePassword seed the dupli1-web service account.
	// When WebServiceEmail is empty, seeding is skipped.
	WebServiceEmail    string
	WebServicePassword string
}
