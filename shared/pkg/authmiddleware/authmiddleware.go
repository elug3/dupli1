// Package authmiddleware is the shared bearer-token HTTP middleware:
// parse the Authorization header, validate it via authjwt, and inject the
// resulting claims into the request context.
//
// cart, order, payment, and notification each carried a byte-identical
// copy of RequireAuth (as a Handler method); product independently built
// a more complete version of the same idea (also with OptionalAuth and a
// permission gate) as a standalone net/http middleware package. Each
// service supplies its own RespondError so the wire-visible error body
// shape a caller sees is unchanged by this move — only the token-parsing
// and validation logic is now shared.
package authmiddleware

import (
	"net/http"
	"strings"

	"github.com/elug3/dupli1/shared/pkg/authjwt"
)

// RespondError writes an error response for the given status and message.
type RespondError func(w http.ResponseWriter, status int, message string)

// RequireAuth returns middleware that requires a valid Bearer access
// token, injecting its claims into the request context (authjwt.WithClaims)
// on success. validator may be nil (auth not configured), in which case
// every request gets a 503 via respondError instead of attempting to
// parse the header.
func RequireAuth(validator authjwt.AccessTokenValidator, respondError RespondError) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if validator == nil {
				respondError(w, http.StatusServiceUnavailable, "auth not configured")
				return
			}
			claims, ok := authenticate(validator, r, w, respondError)
			if !ok {
				return
			}
			next(w, r.WithContext(authjwt.WithClaims(r.Context(), claims)))
		}
	}
}

// OptionalAuth returns middleware that attaches claims when a valid Bearer
// token is present. A missing Authorization header continues
// unauthenticated; a malformed header or an invalid token still fails
// with respondError.
func OptionalAuth(validator authjwt.AccessTokenValidator, respondError RespondError) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "" {
				next(w, r)
				return
			}
			claims, ok := authenticate(validator, r, w, respondError)
			if !ok {
				return
			}
			next(w, r.WithContext(authjwt.WithClaims(r.Context(), claims)))
		}
	}
}

// authenticate parses and validates the Bearer token on r, writing an
// error response and returning ok=false on any failure. The "bearer "
// scheme match is case-insensitive per RFC 7235 (auth-scheme names are
// case-insensitive).
func authenticate(validator authjwt.AccessTokenValidator, r *http.Request, w http.ResponseWriter, respondError RespondError) (claims authjwt.Claims, ok bool) {
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) < 8 || !strings.EqualFold(authHeader[:7], "bearer ") {
		respondError(w, http.StatusUnauthorized, "missing or malformed Authorization header")
		return authjwt.Claims{}, false
	}
	claims, err := validator.ValidateAccessToken(authHeader[7:])
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return authjwt.Claims{}, false
	}
	return claims, true
}
