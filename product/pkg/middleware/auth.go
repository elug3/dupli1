package middleware

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/elug3/dupli1/product/pkg/authjwt"
)

// AccessTokenValidator validates Bearer access tokens and returns claims.
type AccessTokenValidator interface {
	ValidateAccessToken(token string) (authjwt.Claims, error)
}

func RequireAuth(validator AccessTokenValidator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			respondUnauthorized(w)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		claims, err := validator.ValidateAccessToken(tokenStr)
		if err != nil {
			respondUnauthorized(w)
			return
		}

		next.ServeHTTP(w, r.WithContext(authjwt.WithClaims(r.Context(), claims)))
	})
}

// OptionalAuth attaches claims when a valid Bearer token is present.
// Missing Authorization continues unauthenticated; an invalid token returns 401.
func OptionalAuth(validator AccessTokenValidator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(authHeader, "Bearer ") {
			respondUnauthorized(w)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := validator.ValidateAccessToken(tokenStr)
		if err != nil {
			respondUnauthorized(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(authjwt.WithClaims(r.Context(), claims)))
	})
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

func respondUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]interface{}{"error": "unauthorized", "code": 401})
}

func respondForbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	json.NewEncoder(w).Encode(map[string]interface{}{"error": "forbidden: insufficient permission", "code": 403})
}
