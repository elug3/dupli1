package authjwt

import (
	"context"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey struct{}

// Claims holds the verified identity extracted from a JWT.
type Claims struct {
	UserID string
	Roles  []string
}

// Validator validates JWTs and extracts claims.
type Validator struct {
	secret []byte
}

// NewValidator creates a new JWT validator.
func NewValidator(secret string) *Validator {
	return &Validator{secret: []byte(secret)}
}

// Validate parses and validates a JWT, returning its claims.
func (v *Validator) Validate(tokenString string) (Claims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return v.secret, nil
	})
	if err != nil {
		return Claims{}, fmt.Errorf("invalid token: %w", err)
	}
	if !token.Valid {
		return Claims{}, fmt.Errorf("invalid token")
	}

	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return Claims{}, fmt.Errorf("invalid token claims")
	}

	userID, err := extractSubject(mapClaims)
	if err != nil {
		return Claims{}, err
	}

	return Claims{UserID: userID, Roles: extractRoles(mapClaims)}, nil
}

// WithClaims stores claims in the context.
func WithClaims(ctx context.Context, c Claims) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}

// FromContext retrieves claims from the context. Returns false if not present.
func FromContext(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(contextKey{}).(Claims)
	return c, ok
}

// HasRole reports whether any of the given roles is present in the claims.
func (c Claims) HasRole(roles ...string) bool {
	for _, want := range roles {
		for _, have := range c.Roles {
			if have == want {
				return true
			}
		}
	}
	return false
}

func extractSubject(claims jwt.MapClaims) (string, error) {
	if sub, ok := claims["sub"]; ok {
		if s, ok := sub.(string); ok && s != "" {
			return s, nil
		}
	}
	if uid, ok := claims["user_id"]; ok {
		if s, ok := uid.(string); ok && s != "" {
			return s, nil
		}
	}
	return "", fmt.Errorf("subject claim missing")
}

func extractRoles(claims jwt.MapClaims) []string {
	raw, ok := claims["roles"]
	if !ok {
		return []string{}
	}
	slice, ok := raw.([]interface{})
	if !ok {
		return []string{}
	}
	roles := make([]string, 0, len(slice))
	for _, v := range slice {
		if s, ok := v.(string); ok {
			roles = append(roles, s)
		}
	}
	return roles
}
