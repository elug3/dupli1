package httpauth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// APIKeyTokenSource exchanges a service-account API key for access tokens at
// auth's POST /api/v1/auth/token (docs/auth-service-api-keys.md). There is no
// refresh token to rotate or persist — the key is the long-lived credential,
// so when the access token nears expiry it just exchanges again.
type APIKeyTokenSource struct {
	authBaseURL string
	apiKey      string
	client      *http.Client
	skew        time.Duration
	now         func() time.Time

	mu           sync.Mutex
	accessToken  string
	accessExpiry time.Time
}

// NewAPIKeyTokenSource builds a token source for a service-account API key.
func NewAPIKeyTokenSource(authBaseURL, apiKey string, client *http.Client) *APIKeyTokenSource {
	if client == nil {
		client = http.DefaultClient
	}
	return &APIKeyTokenSource{
		authBaseURL: strings.TrimRight(authBaseURL, "/"),
		apiKey:      apiKey,
		client:      client,
		skew:        60 * time.Second,
		now:         time.Now,
	}
}

// Token returns a cached access token, exchanging the key when it is missing
// or within skew of expiring.
func (s *APIKeyTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if s.accessToken != "" && now.Add(s.skew).Before(s.accessExpiry) {
		return s.accessToken, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.authBaseURL+"/api/v1/auth/token", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "ApiKey "+s.apiKey)
	var resp struct {
		Token     string `json:"token"`
		ExpiresIn int    `json:"expires_in"`
	}
	if err := doJSON(s.client, req, &resp); err != nil {
		// Never include the key in errors; auth's body is a fixed
		// "invalid_api_key" or an availability message.
		return "", fmt.Errorf("api key exchange: %w", err)
	}
	if resp.Token == "" {
		return "", fmt.Errorf("api key exchange: missing token")
	}
	s.accessToken = resp.Token
	s.accessExpiry = expiryFromJWT(resp.Token, now)
	return s.accessToken, nil
}

// Invalidate drops the cached access token so the next Token() exchanges again.
func (s *APIKeyTokenSource) Invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accessToken = ""
	s.accessExpiry = time.Time{}
}
