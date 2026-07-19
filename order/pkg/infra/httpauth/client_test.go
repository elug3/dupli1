package httpauth_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elug3/dupli1/order/pkg/infra/httpauth"
)

func TestServiceAccountTokenSource_CachesAndRefreshes(t *testing.T) {
	var logins atomic.Int32
	var refreshes atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/auth/login" && r.Method == http.MethodPost:
			logins.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]string{"refresh_token": "refresh-1"})
		case r.URL.Path == "/api/v1/auth/refresh" && r.Method == http.MethodPost:
			n := refreshes.Add(1)
			exp := time.Now().Add(15 * time.Minute).Unix()
			if n >= 2 {
				exp = time.Now().Add(20 * time.Minute).Unix()
			}
			token := fakeAccessToken(t, exp)
			_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	src := httpauth.NewServiceAccountTokenSource(srv.URL, "order@svc", "secret", srv.Client())

	tok1, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("first Token: %v", err)
	}
	tok2, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("second Token: %v", err)
	}
	if tok1 != tok2 {
		t.Fatal("expected cached access token to be reused")
	}
	if logins.Load() != 1 || refreshes.Load() != 1 {
		t.Fatalf("logins=%d refreshes=%d, want 1/1", logins.Load(), refreshes.Load())
	}

	src.Invalidate()
	tok3, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token after Invalidate: %v", err)
	}
	if tok3 == tok1 {
		t.Fatal("expected new access token after Invalidate")
	}
	if logins.Load() != 1 || refreshes.Load() != 2 {
		t.Fatalf("after invalidate: logins=%d refreshes=%d, want 1/2 (refresh only)", logins.Load(), refreshes.Load())
	}
}

func TestServiceAccountTokenSource_ReloginWhenRefreshFails(t *testing.T) {
	var logins atomic.Int32
	var refreshes atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/auth/login":
			logins.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]string{"refresh_token": "refresh-ok"})
		case r.URL.Path == "/api/v1/auth/refresh":
			n := refreshes.Add(1)
			if n == 1 {
				// First refresh after login succeeds.
				_ = json.NewEncoder(w).Encode(map[string]string{
					"token": fakeAccessToken(t, time.Now().Add(15*time.Minute).Unix()),
				})
				return
			}
			if n == 2 {
				// Simulate expired/revoked refresh token.
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid token"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"token": fakeAccessToken(t, time.Now().Add(15*time.Minute).Unix()),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	src := httpauth.NewServiceAccountTokenSource(srv.URL, "order@svc", "secret", srv.Client())
	if _, err := src.Token(context.Background()); err != nil {
		t.Fatalf("prime: %v", err)
	}
	src.Invalidate()
	if _, err := src.Token(context.Background()); err != nil {
		t.Fatalf("Token after failed refresh: %v", err)
	}
	if logins.Load() != 2 {
		t.Fatalf("logins=%d, want 2 (re-login after refresh failure)", logins.Load())
	}
}

func TestFetchAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			_ = json.NewEncoder(w).Encode(map[string]string{"refresh_token": "r1"})
		case "/api/v1/auth/refresh":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"token": fakeAccessToken(t, time.Now().Add(time.Hour).Unix()),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tok, err := httpauth.FetchAccessToken(context.Background(), srv.URL, "a@b.c", "pw", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tok, ".") {
		t.Fatalf("token = %q", tok)
	}
}

func TestServiceAccountTokenSource_RefreshesBeforeExpirySkew(t *testing.T) {
	var refreshes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			_ = json.NewEncoder(w).Encode(map[string]string{"refresh_token": "r"})
		case "/api/v1/auth/refresh":
			refreshes.Add(1)
			// Token expires in 30s — within the 60s skew, so next Token() must refresh.
			_ = json.NewEncoder(w).Encode(map[string]string{
				"token": fakeAccessToken(t, time.Now().Add(30*time.Second).Unix()),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	src := httpauth.NewServiceAccountTokenSource(srv.URL, "a@b.c", "pw", srv.Client())
	if _, err := src.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if refreshes.Load() != 2 {
		t.Fatalf("refreshes=%d, want 2 (proactive refresh inside skew window)", refreshes.Load())
	}
}

func fakeAccessToken(t *testing.T, exp int64) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]any{"sub": "svc", "type": "access", "exp": exp})
	if err != nil {
		t.Fatal(err)
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}
