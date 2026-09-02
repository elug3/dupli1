package authmiddleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elug3/dupli1/shared/pkg/authjwt"
)

type fakeValidator struct {
	claims authjwt.Claims
	err    error
}

func (v fakeValidator) ValidateAccessToken(token string) (authjwt.Claims, error) {
	if v.err != nil {
		return authjwt.Claims{}, v.err
	}
	return v.claims, nil
}

type recordedError struct {
	status  int
	message string
}

func newRespondError(dst *recordedError) RespondError {
	return func(w http.ResponseWriter, status int, message string) {
		*dst = recordedError{status: status, message: message}
		w.WriteHeader(status)
	}
}

func TestRequireAuth_NilValidatorReturns503(t *testing.T) {
	var got recordedError
	mw := RequireAuth(nil, newRespondError(&got))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	called := false
	mw(func(w http.ResponseWriter, r *http.Request) { called = true })(rec, req)

	if called {
		t.Fatal("next should not be called when validator is nil")
	}
	if got.status != http.StatusServiceUnavailable || got.message != "auth not configured" {
		t.Fatalf("got %+v", got)
	}
}

func TestRequireAuth_MissingHeaderReturns401(t *testing.T) {
	var got recordedError
	mw := RequireAuth(fakeValidator{}, newRespondError(&got))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	mw(func(w http.ResponseWriter, r *http.Request) { t.Fatal("next should not be called") })(rec, req)

	if got.status != http.StatusUnauthorized || got.message != "missing or malformed Authorization header" {
		t.Fatalf("got %+v", got)
	}
}

func TestRequireAuth_MalformedSchemeReturns401(t *testing.T) {
	var got recordedError
	mw := RequireAuth(fakeValidator{}, newRespondError(&got))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	mw(func(w http.ResponseWriter, r *http.Request) { t.Fatal("next should not be called") })(rec, req)

	if got.status != http.StatusUnauthorized || got.message != "missing or malformed Authorization header" {
		t.Fatalf("got %+v", got)
	}
}

func TestRequireAuth_BearerSchemeIsCaseInsensitive(t *testing.T) {
	var got recordedError
	validator := fakeValidator{claims: authjwt.Claims{UserID: "u1"}}
	mw := RequireAuth(validator, newRespondError(&got))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "bearer sometoken")
	called := false
	mw(func(w http.ResponseWriter, r *http.Request) {
		called = true
		claims, ok := authjwt.FromContext(r.Context())
		if !ok || claims.UserID != "u1" {
			t.Fatalf("claims not injected: %+v ok=%v", claims, ok)
		}
	})(rec, req)

	if !called {
		t.Fatal("expected next to be called for a valid lowercase bearer scheme")
	}
	if got != (recordedError{}) {
		t.Fatalf("expected no error response, got %+v", got)
	}
}

func TestRequireAuth_InvalidTokenReturns401(t *testing.T) {
	var got recordedError
	validator := fakeValidator{err: errors.New("bad signature")}
	mw := RequireAuth(validator, newRespondError(&got))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer badtoken")
	mw(func(w http.ResponseWriter, r *http.Request) { t.Fatal("next should not be called") })(rec, req)

	if got.status != http.StatusUnauthorized || got.message != "invalid token" {
		t.Fatalf("got %+v", got)
	}
}

func TestOptionalAuth_MissingHeaderContinuesUnauthenticated(t *testing.T) {
	var got recordedError
	mw := OptionalAuth(fakeValidator{}, newRespondError(&got))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	called := false
	mw(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if _, ok := authjwt.FromContext(r.Context()); ok {
			t.Fatal("expected no claims in context")
		}
	})(rec, req)

	if !called {
		t.Fatal("expected next to be called with no Authorization header")
	}
	if got != (recordedError{}) {
		t.Fatalf("expected no error response, got %+v", got)
	}
}

func TestOptionalAuth_MalformedHeaderStillFails(t *testing.T) {
	var got recordedError
	mw := OptionalAuth(fakeValidator{}, newRespondError(&got))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "garbage")
	mw(func(w http.ResponseWriter, r *http.Request) { t.Fatal("next should not be called") })(rec, req)

	if got.status != http.StatusUnauthorized {
		t.Fatalf("got %+v", got)
	}
}

func TestOptionalAuth_ValidTokenAttachesClaims(t *testing.T) {
	var got recordedError
	validator := fakeValidator{claims: authjwt.Claims{UserID: "u2"}}
	mw := OptionalAuth(validator, newRespondError(&got))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	called := false
	mw(func(w http.ResponseWriter, r *http.Request) {
		called = true
		claims, ok := authjwt.FromContext(r.Context())
		if !ok || claims.UserID != "u2" {
			t.Fatalf("claims not injected: %+v ok=%v", claims, ok)
		}
	})(rec, req)

	if !called {
		t.Fatal("expected next to be called")
	}
}
