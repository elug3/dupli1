package ports

import "context"

// Claims holds the verified identity extracted from a token.
type Claims struct {
	UserID      string
	Permissions []string
}

// Identity is who an access token names beyond its subject. Refresh tokens
// carry none of it — a refresh reloads the user, so a changed account type
// takes effect on the next access token.
type Identity struct {
	Email string
	// AccountType is customer | manager | service.
	AccountType string
	// ServiceName names a service account (e.g. dupli1-order) so internal
	// APIs can check which service is calling. Empty for people.
	ServiceName string
}

// TokenGenerator defines the interface for token generation and validation.
type TokenGenerator interface {
	Generate(ctx context.Context, userID string, permissions []string, id Identity) (string, error)
	Validate(ctx context.Context, token string) (Claims, error)
}
