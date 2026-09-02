package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/authmiddleware"
)

// AccessTokenValidator validates Bearer access tokens and returns claims.
// Alias of the shared interface — see shared/pkg/authjwt.
type AccessTokenValidator = authjwt.AccessTokenValidator

// RequireAuth rejects requests without a valid Bearer access token.
//
// Note: the "bearer " scheme match is now case-insensitive (it delegates to
// shared/pkg/authmiddleware, matching cart/order/payment/notification's
// behavior), where it was previously case-sensitive ("Bearer " only). Auth
// scheme names are case-insensitive per RFC 7235, so this is a compliance
// fix, not a security relaxation — only the token itself is a secret.
func RequireAuth(validator AccessTokenValidator, next http.Handler) http.Handler {
	return authmiddleware.RequireAuth(validator, respondError)(next.ServeHTTP)
}

// OptionalAuth attaches claims when a valid Bearer token is present.
// Missing Authorization continues unauthenticated; an invalid token returns 401.
func OptionalAuth(validator AccessTokenValidator, next http.Handler) http.Handler {
	return authmiddleware.OptionalAuth(validator, respondError)(next.ServeHTTP)
}

// RequireAnyPermission rejects callers who lack any of the given permissions. Must run after RequireAuth.
func RequireAnyPermission(perms ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := authjwt.FromContext(r.Context())
			if !ok || !claims.HasPermission(perms...) {
				respondForbidden(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// respondError preserves product's existing wire format: a fixed
// "unauthorized" message regardless of the specific failure (missing
// header, invalid token, or — new via the shared middleware — an
// unconfigured validator, which previously would have panicked on a nil
// dereference instead of responding at all).
func respondError(w http.ResponseWriter, status int, _ string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{"error": "unauthorized", "code": status})
}

func respondForbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	json.NewEncoder(w).Encode(map[string]interface{}{"error": "forbidden: insufficient permission", "code": 403})
}
