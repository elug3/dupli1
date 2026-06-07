package jwt

import (
	"context"
	"fmt"
	"time"

	"github.com/elug3/schick/pkg/auth/ports"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
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
func (tg *TokenGenerator) Generate(ctx context.Context, userID uuid.UUID, tokenType ports.TokenType, sessionID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"user_id": userID.String(),
		"type":    string(tokenType),
		"exp":     now.Add(tg.expiryDuration).Unix(),
		"iat":     now.Unix(),
	}
	if sessionID != "" {
		claims["session_id"] = sessionID
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(tg.secret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// Validate validates a JWT token and returns the user ID.
func (tg *TokenGenerator) Validate(ctx context.Context, tokenString string, expectedType ports.TokenType) (ports.TokenClaims, error) {
	if err := ctx.Err(); err != nil {
		return ports.TokenClaims{}, err
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Ensure token uses HMAC signing
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(tg.secret), nil
	})
	if err != nil {
		return ports.TokenClaims{}, err
	}

	if !token.Valid {
		return ports.TokenClaims{}, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return ports.TokenClaims{}, fmt.Errorf("invalid token claims")
	}

	raw, ok := claims["user_id"]
	if !ok {
		return ports.TokenClaims{}, fmt.Errorf("user_id claim missing")
	}

	var userID uuid.UUID
	switch v := raw.(type) {
	case string:
		id, err := uuid.Parse(v)
		if err != nil {
			return ports.TokenClaims{}, err
		}
		userID = id
	default:
		return ports.TokenClaims{}, fmt.Errorf("user_id claim has unexpected type")
	}

	rawType, ok := claims["type"].(string)
	if !ok {
		return ports.TokenClaims{}, fmt.Errorf("token type claim missing")
	}
	tokenType := ports.TokenType(rawType)
	if tokenType != expectedType {
		return ports.TokenClaims{}, fmt.Errorf("unexpected token type")
	}

	var sessionID string
	if rawSessionID, ok := claims["session_id"].(string); ok {
		sessionID = rawSessionID
	}

	return ports.TokenClaims{
		UserID:    userID,
		Type:      tokenType,
		SessionID: sessionID,
	}, nil
}
