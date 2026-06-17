package ports

import (
	"context"

	"github.com/google/uuid"
)

type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

type TokenClaims struct {
	UserID    uuid.UUID
	Type      TokenType
	SessionID string
}

// TokenGenerator defines the interface for token generation.
type TokenGenerator interface {
	Generate(ctx context.Context, userID uuid.UUID, tokenType TokenType, sessionID string) (string, error)
	Validate(ctx context.Context, token string, expectedType TokenType) (TokenClaims, error)
}
