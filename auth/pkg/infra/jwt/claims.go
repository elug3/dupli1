package jwt

import (
	"time"

	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/golang-jwt/jwt/v5"
)

func buildMapClaims(userID string, tokenType string, expiry time.Time, userPermissions []string) jwt.MapClaims {
	claims := jwt.MapClaims{
		"sub": userID,
		"exp": expiry.Unix(),
		"iat": time.Now().Unix(),
	}
	if tokenType != "" {
		claims["type"] = tokenType
	}
	if tokenType != "refresh" {
		claims["permissions"] = permissions.Dedupe(userPermissions)
	}
	return claims
}

func claimsFromMap(mapClaims jwt.MapClaims) []string {
	return extractStringSlice(mapClaims, "permissions")
}

func extractStringSlice(mapClaims jwt.MapClaims, key string) []string {
	raw, ok := mapClaims[key]
	if !ok {
		return []string{}
	}
	switch v := raw.(type) {
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	case string:
		if v != "" {
			return []string{v}
		}
	}
	return []string{}
}
