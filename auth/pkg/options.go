package auth

import (
	"fmt"
	"time"
)

type ServerOptions struct {
	// Listen address (e.g. ":8080")
	Addr string

	// Publicly reachable base URL (used for redirect/links)
	PublicAddr string

	// TLS files (optional)
	TLSCertFile string
	TLSKeyFile  string

	// HTTP server timeouts
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration

	// Token settings — provide JWTPrivateKeyFile or JWTPrivateKeyPEM for RS256/JWKS.
	// When neither is set, an ephemeral RSA key is generated on startup (dev only).
	JWTPrivateKeyFile  string // path to PEM-encoded RSA private key
	JWTPrivateKeyPEM   []byte // raw PEM bytes (populated from JWTPrivateKeyFile)
	JWTKeyID           string // "kid" in the JWKS document (default: "default")
	TokenExpiry        time.Duration
	RefreshTokenExpiry time.Duration

	// Cookie settings for session cookies
	CookieName     string
	CookieSecure   bool
	CookieHTTPOnly bool

	// CORS allowed origins
	CORSOrigins []string

	// Persistence/infra
	DBURL    string
	RedisURL string
	NATSURL  string

	// Limits / misc
	MaxConns int
	Debug    bool

	// Logging
	LogOutput string // "json" (default) or "text"
	LogLevel  string // "debug", "info" (default), "warn", "error"

	// OwnerEmail and OwnerPassword seed the initial owner account on first startup.
	OwnerEmail    string
	OwnerPassword string
}

// NewServerOptions returns ServerOptions populated with sensible defaults.
func NewServerOptions() *ServerOptions {
	return &ServerOptions{
		Addr:               ":8080",
		PublicAddr:         "http://localhost:8080",
		ReadTimeout:        5 * time.Second,
		WriteTimeout:       10 * time.Second,
		IdleTimeout:        120 * time.Second,
		ShutdownTimeout:    10 * time.Second,
		TokenExpiry:        15 * time.Minute,
		RefreshTokenExpiry: 24 * time.Hour,
		CookieName:     "schick_session",
		CookieSecure:   true,
		CookieHTTPOnly: true,
		MaxConns:           100,
		Debug:              false,
		LogOutput:          "json",
		LogLevel:           "info",
	}
}

// Validate performs basic sanity checks on the options.
func (o *ServerOptions) Validate() error {
	if o == nil {
		return fmt.Errorf("server options are nil")
	}
	if o.Addr == "" {
		return fmt.Errorf("--addr is required")
	}
	if o.TokenExpiry <= 0 {
		return fmt.Errorf("--token-expiry must be > 0")
	}
	if o.RefreshTokenExpiry <= 0 {
		return fmt.Errorf("RefreshTokenExpiry must be > 0")
	}
	if o.DBURL == "" {
		return fmt.Errorf("--db is required")
	}
	return nil
}
