package ports

import "context"

// Claims holds the verified identity extracted from a token.
type Claims struct {
	UserID      string
	Permissions []string
	Roles       []string // legacy JWT claim during dual-read migration
}

// TokenGenerator defines the interface for token generation and validation.
type TokenGenerator interface {
	Generate(ctx context.Context, userID string, permissions []string) (string, error)
	Validate(ctx context.Context, token string) (Claims, error)
}
