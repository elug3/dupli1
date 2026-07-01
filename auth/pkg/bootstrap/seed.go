package bootstrap

import (
	"context"
	"fmt"

	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/ports"
	"github.com/google/uuid"
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

	u, err := domain.NewUser(uuid.New().String(), cfg.OwnerEmail, cfg.OwnerPassword, domain.RoleOwner, domain.RoleProductManager)
	if err != nil {
		return fmt.Errorf("seed owner: create: %w", err)
	}
	if err := repo.Save(ctx, u); err != nil {
		return fmt.Errorf("seed owner: save: %w", err)
	}

	cfg.Logger.Info().Str("event", "owner_seeded").Str("email", cfg.OwnerEmail).Msg("owner user seeded")
	return nil
}
