package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/elug3/schick/pkg/auth/autherrors"
	"github.com/elug3/schick/pkg/auth/domain"
	"github.com/elug3/schick/pkg/auth/handler"
	jwtgen "github.com/elug3/schick/pkg/auth/infra/jwt"
	"github.com/elug3/schick/pkg/auth/infra/memory"
	"github.com/elug3/schick/pkg/auth/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ---- in-memory UserRepository fake ----------------------------------------

type fakeUserRepo struct {
	mu      sync.RWMutex
	byID    map[uuid.UUID]*domain.User
	byEmail map[string]*domain.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{
		byID:    make(map[uuid.UUID]*domain.User),
		byEmail: make(map[string]*domain.User),
	}
}

func (r *fakeUserRepo) Save(_ context.Context, u *domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.byEmail[u.Email]; ok && existing.ID != u.ID {
		return autherrors.ErrUserAlreadyExists
	}
	// Remove stale email index when updating an existing user.
	if old, ok := r.byID[u.ID]; ok && old.Email != u.Email {
		delete(r.byEmail, old.Email)
	}
	r.byID[u.ID] = u
	r.byEmail[u.Email] = u
	return nil
}

func (r *fakeUserRepo) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u := r.byEmail[email]
	return u, nil
}

func (r *fakeUserRepo) FindByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u := r.byID[id]
	return u, nil
}

func (r *fakeUserRepo) ListUsers(_ context.Context) ([]*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	users := make([]*domain.User, 0, len(r.byID))
	for _, user := range r.byID {
		users = append(users, user)
	}
	return users, nil
}

func (r *fakeUserRepo) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.byID[id]; ok {
		delete(r.byEmail, u.Email)
		delete(r.byID, id)
	}
	return nil
}

// ---- test stack ------------------------------------------------------------

type stack struct {
	repo   *fakeUserRepo
	router *gin.Engine
}

func newStack(t *testing.T) *stack {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := newFakeUserRepo()
	accessGen := jwtgen.NewTokenGenerator("test-access-secret", 900)
	refreshGen := jwtgen.NewTokenGenerator("test-refresh-secret", 86400)
	sessions := memory.NewSessionStore()

	svc := service.NewService(repo, accessGen, refreshGen, sessions, 24*time.Hour)
	h := handler.NewHandler(svc)

	r := gin.New()
	r.Use(gin.Recovery())
	v1 := r.Group("/api/v1/auth")
	{
		v1.POST("/register", h.Register)
		v1.POST("/login", h.Login)
		v1.POST("/logout", h.Logout)
		v1.GET("/me", h.Me)
		v1.POST("/refresh", h.Refresh)
	}

	return &stack{repo: repo, router: r}
}

func (s *stack) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

func (s *stack) doWithAuth(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

// registerAndLogin is a test helper that creates a user and returns its tokens.
func (s *stack) registerAndLogin(t *testing.T, email, password string) (accessToken, refreshToken string) {
	t.Helper()

	w := s.do(t, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email":    email,
		"password": password,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("register: want 201, got %d: %s", w.Code, w.Body.String())
	}

	w = s.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email":    email,
		"password": password,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login: want 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return resp.AccessToken, resp.RefreshToken
}

// ---- POST /register --------------------------------------------------------

func TestRegister(t *testing.T) {
	tests := []struct {
		name     string
		body     any
		wantCode int
	}{
		{
			name:     "valid",
			body:     map[string]string{"email": "alice@example.com", "password": "supersecret"},
			wantCode: http.StatusCreated,
		},
		{
			name:     "invalid email",
			body:     map[string]string{"email": "not-an-email", "password": "supersecret"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "password too short",
			body:     map[string]string{"email": "bob@example.com", "password": "short"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing email",
			body:     map[string]string{"password": "supersecret"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing password",
			body:     map[string]string{"email": "carol@example.com"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "bad JSON",
			body:     nil, // no Content-Type, empty body → binding error
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newStack(t)
			w := s.do(t, http.MethodPost, "/api/v1/auth/register", tc.body)
			if w.Code != tc.wantCode {
				t.Errorf("want %d, got %d: %s", tc.wantCode, w.Code, w.Body.String())
			}
		})
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	s := newStack(t)
	body := map[string]string{"email": "dup@example.com", "password": "supersecret"}

	w := s.do(t, http.MethodPost, "/api/v1/auth/register", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("first register: want 201, got %d", w.Code)
	}

	w = s.do(t, http.MethodPost, "/api/v1/auth/register", body)
	if w.Code != http.StatusConflict {
		t.Errorf("duplicate register: want 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRegister_ResponseContainsUserID(t *testing.T) {
	s := newStack(t)
	w := s.do(t, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": "id@example.com", "password": "supersecret",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d", w.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, err := uuid.Parse(resp["user_id"]); err != nil {
		t.Errorf("user_id %q is not a valid UUID: %v", resp["user_id"], err)
	}
}

// ---- POST /login -----------------------------------------------------------

func TestLogin(t *testing.T) {
	s := newStack(t)
	const email, password = "login@example.com", "supersecret"

	w := s.do(t, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": email, "password": password,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("setup register: %d", w.Code)
	}

	t.Run("valid credentials", func(t *testing.T) {
		w := s.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
			"email": email, "password": password,
		})
		if w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.AccessToken == "" {
			t.Error("access_token is empty")
		}
		if resp.RefreshToken == "" {
			t.Error("refresh_token is empty")
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		w := s.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
			"email": email, "password": "wrongpassword",
		})
		if w.Code != http.StatusUnauthorized {
			t.Errorf("want 401, got %d", w.Code)
		}
	})

	t.Run("unknown email", func(t *testing.T) {
		w := s.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
			"email": "nobody@example.com", "password": password,
		})
		if w.Code != http.StatusUnauthorized {
			t.Errorf("want 401, got %d", w.Code)
		}
	})

	t.Run("missing password field", func(t *testing.T) {
		w := s.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
			"email": email,
		})
		if w.Code != http.StatusBadRequest {
			t.Errorf("want 400, got %d", w.Code)
		}
	})

	t.Run("bad JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("want 400, got %d", w.Code)
		}
	})
}

// ---- GET /me ---------------------------------------------------------------

func TestMe(t *testing.T) {
	s := newStack(t)
	const email, password = "me@example.com", "supersecret"
	accessToken, _ := s.registerAndLogin(t, email, password)

	t.Run("valid token", func(t *testing.T) {
		w := s.doWithAuth(t, http.MethodGet, "/api/v1/auth/me", accessToken, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Email != email {
			t.Errorf("email: got %q, want %q", resp.Email, email)
		}
		if _, err := uuid.Parse(resp.ID); err != nil {
			t.Errorf("id %q is not a valid UUID", resp.ID)
		}
	})

	t.Run("missing authorization header", func(t *testing.T) {
		w := s.do(t, http.MethodGet, "/api/v1/auth/me", nil)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("want 401, got %d", w.Code)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		w := s.doWithAuth(t, http.MethodGet, "/api/v1/auth/me", "this.is.garbage", nil)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("want 401, got %d", w.Code)
		}
	})

	t.Run("refresh token used as access token", func(t *testing.T) {
		_, refreshToken := s.registerAndLogin(t, "me2@example.com", "supersecret")
		w := s.doWithAuth(t, http.MethodGet, "/api/v1/auth/me", refreshToken, nil)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("want 401, got %d: token type mismatch should be rejected", w.Code)
		}
	})

	t.Run("malformed bearer header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.Header.Set("Authorization", accessToken) // missing "Bearer " prefix
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("want 401, got %d", w.Code)
		}
	})
}

// ---- POST /logout ----------------------------------------------------------

func TestLogout(t *testing.T) {
	s := newStack(t)

	t.Run("valid refresh token", func(t *testing.T) {
		_, refreshToken := s.registerAndLogin(t, "logout@example.com", "supersecret")
		w := s.do(t, http.MethodPost, "/api/v1/auth/logout", map[string]string{
			"refresh_token": refreshToken,
		})
		if w.Code != http.StatusNoContent {
			t.Errorf("want 204, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		w := s.do(t, http.MethodPost, "/api/v1/auth/logout", map[string]string{
			"refresh_token": "invalid.token.value",
		})
		if w.Code != http.StatusUnauthorized {
			t.Errorf("want 401, got %d", w.Code)
		}
	})

	t.Run("already logged out", func(t *testing.T) {
		_, refreshToken := s.registerAndLogin(t, "logout2@example.com", "supersecret")

		w := s.do(t, http.MethodPost, "/api/v1/auth/logout", map[string]string{
			"refresh_token": refreshToken,
		})
		if w.Code != http.StatusNoContent {
			t.Fatalf("first logout: want 204, got %d", w.Code)
		}

		w = s.do(t, http.MethodPost, "/api/v1/auth/logout", map[string]string{
			"refresh_token": refreshToken,
		})
		if w.Code != http.StatusUnauthorized {
			t.Errorf("second logout: want 401, got %d", w.Code)
		}
	})

	t.Run("missing refresh_token field", func(t *testing.T) {
		w := s.do(t, http.MethodPost, "/api/v1/auth/logout", map[string]string{})
		if w.Code != http.StatusBadRequest {
			t.Errorf("want 400, got %d", w.Code)
		}
	})
}

// ---- POST /refresh ---------------------------------------------------------

func TestRefresh(t *testing.T) {
	s := newStack(t)

	t.Run("valid refresh token returns new token pair", func(t *testing.T) {
		_, refreshToken := s.registerAndLogin(t, "refresh@example.com", "supersecret")

		w := s.do(t, http.MethodPost, "/api/v1/auth/refresh", map[string]string{
			"refresh_token": refreshToken,
		})
		if w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.AccessToken == "" {
			t.Error("access_token is empty")
		}
		if resp.RefreshToken == "" {
			t.Error("refresh_token is empty")
		}
		if resp.RefreshToken == refreshToken {
			t.Error("refresh token was not rotated")
		}
	})

	t.Run("token rotation: old token rejected after refresh", func(t *testing.T) {
		_, refreshToken := s.registerAndLogin(t, "refresh2@example.com", "supersecret")

		w := s.do(t, http.MethodPost, "/api/v1/auth/refresh", map[string]string{
			"refresh_token": refreshToken,
		})
		if w.Code != http.StatusOK {
			t.Fatalf("first refresh: want 200, got %d", w.Code)
		}

		// The original refresh token must now be invalid.
		w = s.do(t, http.MethodPost, "/api/v1/auth/refresh", map[string]string{
			"refresh_token": refreshToken,
		})
		if w.Code != http.StatusUnauthorized {
			t.Errorf("reuse of rotated token: want 401, got %d", w.Code)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		w := s.do(t, http.MethodPost, "/api/v1/auth/refresh", map[string]string{
			"refresh_token": "bad.token.value",
		})
		if w.Code != http.StatusUnauthorized {
			t.Errorf("want 401, got %d", w.Code)
		}
	})

	t.Run("access token used as refresh token", func(t *testing.T) {
		accessToken, _ := s.registerAndLogin(t, "refresh3@example.com", "supersecret")
		w := s.do(t, http.MethodPost, "/api/v1/auth/refresh", map[string]string{
			"refresh_token": accessToken,
		})
		if w.Code != http.StatusUnauthorized {
			t.Errorf("want 401, got %d: token type mismatch should be rejected", w.Code)
		}
	})

	t.Run("missing refresh_token field", func(t *testing.T) {
		w := s.do(t, http.MethodPost, "/api/v1/auth/refresh", map[string]string{})
		if w.Code != http.StatusBadRequest {
			t.Errorf("want 400, got %d", w.Code)
		}
	})
}
