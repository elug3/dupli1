package httpauth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TokenSource supplies Bearer access tokens for outbound service calls.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// StaticToken returns a fixed bearer token (e.g. DUPLI1_INVENTORY_BEARER_TOKEN).
type StaticToken string

func (s StaticToken) Token(context.Context) (string, error) {
	return string(s), nil
}

// ServiceAccountTokenSource logs in once, caches the short-lived access token,
// and refreshes it before expiry (re-login if the refresh token is rejected).
type ServiceAccountTokenSource struct {
	authBaseURL string
	email       string
	password    string
	client      *http.Client
	skew        time.Duration
	now         func() time.Time

	mu           sync.Mutex
	accessToken  string
	accessExpiry time.Time
	refreshToken string
}

// NewServiceAccountTokenSource builds a refreshing token source for a service account.
func NewServiceAccountTokenSource(authBaseURL, email, password string, client *http.Client) *ServiceAccountTokenSource {
	if client == nil {
		client = http.DefaultClient
	}
	return &ServiceAccountTokenSource{
		authBaseURL: strings.TrimRight(authBaseURL, "/"),
		email:       email,
		password:    password,
		client:      client,
		skew:        60 * time.Second,
		now:         time.Now,
	}
}

// Token returns a valid access token, refreshing or re-logging in as needed.
func (s *ServiceAccountTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokenLocked(ctx, false)
}

// Invalidate clears the cached access token so the next Token() refreshes.
// Used after an outbound 401 so a stale token is not reused.
func (s *ServiceAccountTokenSource) Invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accessToken = ""
	s.accessExpiry = time.Time{}
}

func (s *ServiceAccountTokenSource) tokenLocked(ctx context.Context, forceRefresh bool) (string, error) {
	now := s.now()
	if !forceRefresh && s.accessToken != "" && now.Add(s.skew).Before(s.accessExpiry) {
		return s.accessToken, nil
	}

	if s.refreshToken != "" {
		if err := s.refreshLocked(ctx); err == nil {
			return s.accessToken, nil
		}
		// Refresh failed — drop credentials and re-login.
		s.refreshToken = ""
		s.accessToken = ""
		s.accessExpiry = time.Time{}
	}

	if err := s.loginLocked(ctx); err != nil {
		return "", err
	}
	if err := s.refreshLocked(ctx); err != nil {
		return "", err
	}
	return s.accessToken, nil
}

func (s *ServiceAccountTokenSource) loginLocked(ctx context.Context) error {
	var loginResp struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := postJSON(ctx, s.client, s.authBaseURL+"/api/v1/auth/login", map[string]string{
		"email":    s.email,
		"password": s.password,
	}, &loginResp); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if loginResp.RefreshToken == "" {
		return fmt.Errorf("login: missing refresh_token")
	}
	s.refreshToken = loginResp.RefreshToken
	return nil
}

func (s *ServiceAccountTokenSource) refreshLocked(ctx context.Context) error {
	var refreshResp struct {
		Token string `json:"token"`
	}
	if err := postJSON(ctx, s.client, s.authBaseURL+"/api/v1/auth/refresh", map[string]string{
		"refresh_token": s.refreshToken,
	}, &refreshResp); err != nil {
		return fmt.Errorf("refresh: %w", err)
	}
	if refreshResp.Token == "" {
		return fmt.Errorf("refresh: missing token")
	}
	s.accessToken = refreshResp.Token
	s.accessExpiry = expiryFromJWT(refreshResp.Token, s.now())
	return nil
}

// FetchAccessToken logs in and refreshes once (legacy one-shot helper).
func FetchAccessToken(ctx context.Context, authBaseURL, email, password string, client *http.Client) (string, error) {
	src := NewServiceAccountTokenSource(authBaseURL, email, password, client)
	return src.Token(ctx)
}

func expiryFromJWT(token string, now time.Time) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return now.Add(14 * time.Minute)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return now.Add(14 * time.Minute)
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp <= 0 {
		return now.Add(14 * time.Minute)
	}
	return time.Unix(claims.Exp, 0)
}

func postJSON(ctx context.Context, client *http.Client, url string, body any, target any) error {
	var payload bytes.Buffer
	if err := json.NewEncoder(&payload).Encode(body); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &payload)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		if errBody.Error == "" {
			errBody.Error = resp.Status
		}
		return fmt.Errorf("%s", errBody.Error)
	}
	if target != nil {
		return json.NewDecoder(resp.Body).Decode(target)
	}
	return nil
}
