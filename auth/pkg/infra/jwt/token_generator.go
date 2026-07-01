package jwt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/elug3/dupli1/auth/pkg/autherrors"
	"github.com/elug3/dupli1/auth/pkg/ports"
	"github.com/golang-jwt/jwt/v5"
)

// TokenGenerator implements ports.TokenGenerator using JWT.
type TokenGenerator struct {
	secret         string
	expiryDuration time.Duration
	tokenType      string
}

// NewTokenGenerator creates a new JWT token generator.
func NewTokenGenerator(secret string, expirySeconds int64) *TokenGenerator {
	return NewTokenGeneratorWithType(secret, expirySeconds, "access")
}

// NewTokenGeneratorWithType creates a JWT token generator that stamps a type claim.
func NewTokenGeneratorWithType(secret string, expirySeconds int64, tokenType string) *TokenGenerator {
	return &TokenGenerator{
		secret:         secret,
		expiryDuration: time.Duration(expirySeconds) * time.Second,
		tokenType:      tokenType,
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
	if tg.tokenType != "" {
		claims["type"] = tg.tokenType
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(tg.secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}

	return tokenString, nil
}

// Validate validates a JWT token and returns the claims.
// Returns autherrors.ErrTokenExpired when the token has expired,
// and autherrors.ErrInvalidToken for any other validation failure.
func (tg *TokenGenerator) Validate(ctx context.Context, tokenString string) (ports.Claims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(tg.secret), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return ports.Claims{}, autherrors.ErrTokenExpired
		}
		return ports.Claims{}, autherrors.ErrInvalidToken
	}

	if !token.Valid {
		return ports.Claims{}, autherrors.ErrInvalidToken
	}

	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return ports.Claims{}, autherrors.ErrInvalidToken
	}

	userID, err := extractSubject(mapClaims)
	if err != nil {
		return ports.Claims{}, autherrors.ErrInvalidToken
	}

	if err := validateTokenType(mapClaims, tg.tokenType); err != nil {
		return ports.Claims{}, autherrors.ErrInvalidToken
	}

	roles := extractRoles(mapClaims)

	return ports.Claims{UserID: userID, Roles: roles}, nil
}

func validateTokenType(claims jwt.MapClaims, expected string) error {
	if expected == "" {
		return nil
	}
	raw, ok := claims["type"]
	if !ok {
		return fmt.Errorf("token type claim missing")
	}
	typ, ok := raw.(string)
	if !ok || typ != expected {
		return fmt.Errorf("unexpected token type %q, want %q", typ, expected)
	}
	return nil
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
