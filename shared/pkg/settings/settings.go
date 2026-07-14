// Package settings provides a shared non-secret service settings response
// for Dupli1 HTTP services (GET /settings and GET /api/v1/<service>/settings).
package settings

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const APIVersion = "v1"

// Response is the public, read-only settings payload. Never include secrets,
// passwords, private keys, full DSNs, or API tokens.
type Response struct {
	Service      string                `json:"service"`
	APIVersion   string                `json:"api_version"`
	Auth         *AuthInfo             `json:"auth,omitempty"`
	Storage      string                `json:"storage,omitempty"` // "postgres" or "memory"
	Features     map[string]bool       `json:"features,omitempty"`
	Limits       map[string]any        `json:"limits,omitempty"`
	Dependencies map[string]Dependency `json:"dependencies,omitempty"`
}

// AuthInfo describes how the service validates JWTs (no secrets).
type AuthInfo struct {
	Mode              string `json:"mode"` // "jwks", "hs256", "rs256", "none"
	JWKSConfigured    bool   `json:"jwks_configured"`
	JWTSecretFallback bool   `json:"jwt_secret_fallback,omitempty"`
}

// Dependency reports whether an outbound dependency is configured.
// Host is the hostname only (no credentials or path).
type Dependency struct {
	Configured bool   `json:"configured"`
	Host       string `json:"host,omitempty"`
}

// NewResponse returns a Response with service name and API version set.
func NewResponse(service string) Response {
	return Response{
		Service:    service,
		APIVersion: APIVersion,
	}
}

// HostFromURL returns the hostname from a URL and whether the URL is non-empty.
// Credentials and paths are never returned.
func HostFromURL(raw string) (host string, configured bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		// Accept bare host:port values.
		if h, _, ok := strings.Cut(raw, "/"); ok && h != "" {
			return stripUserinfo(h), true
		}
		return stripUserinfo(raw), true
	}
	return u.Hostname(), true
}

func stripUserinfo(host string) string {
	if i := strings.LastIndex(host, "@"); i >= 0 {
		return host[i+1:]
	}
	return host
}

// Dep builds a Dependency from a URL string.
func Dep(rawURL string) Dependency {
	host, ok := HostFromURL(rawURL)
	return Dependency{Configured: ok, Host: host}
}

// StorageMode reports "postgres" when a DB URL is set, otherwise "memory".
func StorageMode(dbURL string) string {
	if strings.TrimSpace(dbURL) == "" {
		return "memory"
	}
	return "postgres"
}

// ConsumerAuth describes JWT validation for services that consume access tokens
// via JWKS and/or HS256 secret fallback.
func ConsumerAuth(jwksURL, jwtSecret string) *AuthInfo {
	jwks := strings.TrimSpace(jwksURL) != ""
	secret := strings.TrimSpace(jwtSecret) != ""
	mode := "none"
	switch {
	case jwks:
		mode = "jwks"
	case secret:
		mode = "hs256"
	}
	return &AuthInfo{
		Mode:              mode,
		JWKSConfigured:    jwks,
		JWTSecretFallback: secret,
	}
}

// TimeoutSeconds converts a duration to whole seconds for limits maps.
func TimeoutSeconds(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	return int(d.Seconds())
}

// WriteJSON writes v as application/json with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Handler returns a GET-only stdlib handler that serves the given response.
func Handler(resp Response) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		WriteJSON(w, http.StatusOK, resp)
	}
}
