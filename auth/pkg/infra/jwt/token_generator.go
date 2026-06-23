package jwt

import (
	"context"
	"fmt"
	"time"

	"github.com/elug3/schick/auth/pkg/ports"
	"github.com/golang-jwt/jwt/v5"
)

// TokenGenerator implements ports.TokenGenerator using JWT.
type TokenGenerator struct {
	secret         string
	expiryDuration time.Duration
}

// NewTokenGenerator creates a new JWT token generator.
func NewTokenGenerator(secret string, expirySeconds int64) *TokenGenerator {
	return &TokenGenerator{
		secret:         secret,
		expiryDuration: time.Duration(expirySeconds) * time.Second,
	}
}

// Generate generates a JWT token for a user with their roles.
func (tg *TokenGenerator) Generate(ctx context.Context, userID string, roles []string) (string, error) {
	if roles == nil {
		roles = []string{}
	}
	claims := jwt.MapClaims{
		"sub":   userID,
		"roles": roles,
		"exp":   time.Now().Add(tg.expiryDuration).Unix(),
		"iat":   time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(tg.secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}

	return tokenString, nil
}

// Validate validates a JWT token and returns the claims.
func (tg *TokenGenerator) Validate(ctx context.Context, tokenString string) (ports.Claims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(tg.secret), nil
	})
	if err != nil {
		return ports.Claims{}, fmt.Errorf("parse token: %w", err)
	}

	if !token.Valid {
		return ports.Claims{}, fmt.Errorf("invalid token")
	}

	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return ports.Claims{}, fmt.Errorf("invalid token claims")
	}

	userID, err := extractSubject(mapClaims)
	if err != nil {
		return ports.Claims{}, err
	}

	roles := extractRoles(mapClaims)

	return ports.Claims{UserID: userID, Roles: roles}, nil
}

func extractSubject(claims jwt.MapClaims) (string, error) {
	// prefer standard "sub" claim, fall back to legacy "user_id"
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
