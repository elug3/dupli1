package ports

import (
	"context"
	"time"

	"github.com/elug3/dupli1/auth/pkg/domain"
)

// UserRepository defines persistence operations for users.
type UserRepository interface {
	// FindByEmail returns a user by email or (nil, nil) when not found.
	FindByEmail(ctx context.Context, email string) (*domain.User, error)

	// FindByID returns a user by ID or (nil, nil) when not found.
	FindByID(ctx context.Context, id string) (*domain.User, error)

	// ListAll returns all users ordered by creation time.
	ListAll(ctx context.Context) ([]*domain.User, error)

	// Save creates or updates a user.
	Save(ctx context.Context, u *domain.User) error

	// Delete removes a user by id.
	Delete(ctx context.Context, id string) error
}

// RegistrationStats reads sign-up times for the registrations report.
type RegistrationStats interface {
	// RegistrationTimes returns the creation time of every accountType
	// account created in [start, end), and how many accountType accounts
	// have no recorded creation time at all.
	RegistrationTimes(ctx context.Context, accountType string, start, end time.Time) ([]time.Time, int, error)
}
