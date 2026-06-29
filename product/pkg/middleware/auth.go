package middleware

import (
	"encoding/json"
	"net/http"
	"strings"
)

// AccessTokenValidator validates Bearer access tokens.
type AccessTokenValidator interface {
	ValidateAccessToken(token string) error
}

func RequireAuth(validator AccessTokenValidator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			respondUnauthorized(w)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		if err := validator.ValidateAccessToken(tokenStr); err != nil {
			respondUnauthorized(w)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func respondUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]interface{}{"error": "unauthorized", "code": 401})
}
