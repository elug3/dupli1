package middleware

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

func RequireAuth(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			respondUnauthorized(w)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(secret), nil
		})
		if err != nil || !token.Valid {
			respondUnauthorized(w)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			respondUnauthorized(w)
			return
		}

		if tokenType, _ := claims["type"].(string); tokenType != "access" {
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
