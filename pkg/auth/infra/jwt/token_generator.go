package jwt

import (
	"context"
	"fmt"
	"strconv"
	"time"

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

// Generate generates a JWT token for a user.
func (tg *TokenGenerator) Generate(ctx context.Context, userID string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(tg.expiryDuration).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(tg.secret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// Validate validates a JWT token and returns the user ID.
func (tg *TokenGenerator) Validate(ctx context.Context, tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Ensure token uses HMAC signing
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(tg.secret), nil
	})
	if err != nil {
		return "", err
	}

	if !token.Valid {
		return "", fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid token claims")
	}

	raw, ok := claims["user_id"]
	if !ok {
		return "", fmt.Errorf("user_id claim missing")
	}

	switch v := raw.(type) {
	case string:
		return v, nil
	case float64:
		// handle numeric user ids encoded as numbers
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		return "", fmt.Errorf("user_id claim has unexpected type")
	}
}
