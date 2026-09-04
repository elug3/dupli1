package ports

import (
	"context"
	"errors"

	"github.com/elug3/dupli1/profile/pkg/domain"
)

var ErrAddressNotFound = errors.New("address not found")

// ProfileRepository persists customer profiles and saved addresses.
type ProfileRepository interface {
	GetProfile(ctx context.Context, userID string) (*domain.Profile, error)
	UpsertProfile(ctx context.Context, profile *domain.Profile) error

	ListAddresses(ctx context.Context, userID string) ([]*domain.Address, error)
	CountAddresses(ctx context.Context, userID string) (int, error)
	GetAddress(ctx context.Context, userID, addressID string) (*domain.Address, error)
	SaveAddress(ctx context.Context, address *domain.Address) error
	DeleteAddress(ctx context.Context, userID, addressID string) error
	ClearDefaultAddresses(ctx context.Context, userID string) error
	NextAddressID(ctx context.Context) (string, error)

	// DeleteUserData permanently removes all addresses and the profile row
	// for userID. Called when the owning user account is deleted (see
	// shared/pkg/events.UserDeleted) since profile owns no foreign key to
	// the users table and won't be cleaned up by a DB cascade.
	DeleteUserData(ctx context.Context, userID string) error
}
