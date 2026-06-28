// Tests for newRouter's JWKS endpoint registration and response.
package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	jwtinfra "github.com/elug3/schick/auth/pkg/infra/jwt"
)

func buildTestJWKS(t *testing.T) []byte {
	t.Helper()
	key, err := jwtinfra.GenerateRSAKey(2048)
	if err != nil {
		t.Fatalf("GenerateRSAKey: %v", err)
	}
	gen := jwtinfra.NewRSATokenGenerator(key, "test-kid", 3600)
	data, err := json.Marshal(gen.PublicJWKS())
	if err != nil {
		t.Fatalf("marshal JWKS: %v", err)
	}
	return data
}

func TestJWKSEndpoint_RootPath(t *testing.T) {
	jwksJSON := buildTestJWKS(t)
	r := newRouter(nil, false, jwksJSON)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if w.Body.String() != string(jwksJSON) {
		t.Errorf("body = %s, want %s", w.Body.String(), jwksJSON)
	}
}

func TestJWKSEndpoint_PrefixedPath(t *testing.T) {
	jwksJSON := buildTestJWKS(t)
	r := newRouter(nil, false, jwksJSON)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/.well-known/jwks.json", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != string(jwksJSON) {
		t.Errorf("body = %s, want %s", w.Body.String(), jwksJSON)
	}
}

func TestJWKSEndpoint_ResponseIsValidJSON(t *testing.T) {
	jwksJSON := buildTestJWKS(t)
	r := newRouter(nil, false, jwksJSON)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil)
	r.ServeHTTP(w, req)

	var doc jwtinfra.JWKS
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("response is not valid JWKS JSON: %v", err)
	}
	if len(doc.Keys) == 0 {
		t.Fatal("JWKS response contains no keys")
	}
	k := doc.Keys[0]
	if k.Kty != "RSA" || k.Alg != "RS256" || k.Kid != "test-kid" {
		t.Errorf("unexpected key fields: kty=%q alg=%q kid=%q", k.Kty, k.Alg, k.Kid)
	}
}

func TestJWKSEndpoint_NotRegisteredWhenNoKey(t *testing.T) {
	// When jwksJSON is nil (HMAC mode), the well-known routes must not exist.
	r := newRouter(nil, false, nil)

	for _, path := range []string{
		"/.well-known/jwks.json",
		"/api/v1/auth/.well-known/jwks.json",
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("path %s: status = %d, want 404", path, w.Code)
		}
	}
}

func TestJWKSEndpoint_BothPathsReturnIdenticalBody(t *testing.T) {
	jwksJSON := buildTestJWKS(t)
	r := newRouter(nil, false, jwksJSON)

	get := func(path string) string {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w.Body.String()
	}

	root := get("/.well-known/jwks.json")
	prefixed := get("/api/v1/auth/.well-known/jwks.json")
	if root != prefixed {
		t.Errorf("responses differ:\n  root:     %s\n  prefixed: %s", root, prefixed)
	}
}
