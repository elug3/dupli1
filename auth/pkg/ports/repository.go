package ports

import (
	"context"

	"github.com/elug3/schick/auth/pkg/domain"
)

// UserRepository defines persistence operations for users.
type UserRepository interface {
	// FindByEmail returns a user by email or (nil, nil) when not found.
	FindByEmail(ctx context.Context, email string) (*domain.User, error)

	// FindByID returns a user by ID or (nil, nil) when not found.
	FindByID(ctx context.Context, id string) (*domain.User, error)

	// Save creates or updates a user.
	Save(ctx context.Context, u *domain.User) error

	// Delete removes a user by id.
	Delete(ctx context.Context, id string) error

	// ListAll returns all users.
	ListAll(ctx context.Context) ([]*domain.User, error)
}
