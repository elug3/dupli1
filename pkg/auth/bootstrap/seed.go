package bootstrap

import (
	"context"
	"fmt"

	"github.com/elug3/schick/pkg/auth/domain"
	"github.com/elug3/schick/pkg/auth/ports"
)

// seedOwner creates the initial owner user if one does not already exist.
// It is idempotent: repeated calls with the same email are no-ops.
func seedOwner(ctx context.Context, cfg Config, repo ports.UserRepository) error {
	if cfg.OwnerEmail == "" {
		return nil
	}

	existing, err := repo.FindByEmail(ctx, cfg.OwnerEmail)
	if err != nil {
		return fmt.Errorf("seed owner: lookup: %w", err)
	}
	if existing != nil {
		return nil
	}

	u := domain.NewUser(cfg.OwnerEmail)
	u.Role = "owner"
	if err := u.SetPassword(cfg.OwnerPassword); err != nil {
		return fmt.Errorf("seed owner: hash password: %w", err)
	}
	if err := repo.Save(ctx, u); err != nil {
		return fmt.Errorf("seed owner: save: %w", err)
	}

	fmt.Printf("owner user seeded: %s\n", cfg.OwnerEmail)
	return nil
}
