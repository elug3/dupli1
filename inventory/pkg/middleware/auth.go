package middleware

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/elug3/dupli1/inventory/pkg/authjwt"
)

// AccessTokenValidator validates Bearer access tokens and returns claims.
type AccessTokenValidator interface {
	ValidateAccessToken(token string) (authjwt.Claims, error)
}

func RequireAuth(validator AccessTokenValidator, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if validator == nil {
			next(w, r)
			return
		}

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

		next(w, r.WithContext(authjwt.WithClaims(r.Context(), claims)))
	}
}

func RequireAnyPermission(perms ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			claims, ok := authjwt.FromContext(r.Context())
			if !ok || !claims.HasPermission(perms...) {
				respondForbidden(w)
				return
			}
			next(w, r)
		}
	}
}

func respondUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": "unauthorized", "code": 401})
}

func respondForbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": "forbidden: insufficient permission", "code": 403})
}
