package ports

import "context"

// TokenGenerator defines the interface for token generation.
type TokenGenerator interface {
	Generate(ctx context.Context, userID string) (string, error)
	Validate(ctx context.Context, token string) (string, error) // returns userID
}
