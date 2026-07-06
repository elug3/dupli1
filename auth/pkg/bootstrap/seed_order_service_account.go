package bootstrap

import (
	"context"
	"fmt"

	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/ports"
	"github.com/google/uuid"
)

// seedOrderServiceAccount creates the dupli1-order service account when configured.
func seedOrderServiceAccount(ctx context.Context, cfg Config, repo ports.UserRepository) error {
	if cfg.OrderServiceEmail == "" {
		return nil
	}
	if cfg.OrderServicePassword == "" {
		return fmt.Errorf("seed order service account: DUPLI1_ORDER_SERVICE_PASSWORD is required when DUPLI1_ORDER_SERVICE_EMAIL is set")
	}

	existing, err := repo.FindByEmail(ctx, cfg.OrderServiceEmail)
	if err != nil {
		return fmt.Errorf("seed order service account: lookup: %w", err)
	}
	if existing != nil {
		return nil
	}

	u, err := domain.NewUser(
		uuid.New().String(),
		cfg.OrderServiceEmail,
		cfg.OrderServicePassword,
		domain.AccountTypeService,
		domain.RoleOrderManager,
	)
	if err != nil {
		return fmt.Errorf("seed order service account: create: %w", err)
	}
	if err := repo.Save(ctx, u); err != nil {
		return fmt.Errorf("seed order service account: save: %w", err)
	}

	cfg.Logger.Info().
		Str("event", "order_service_account_seeded").
		Str("email", cfg.OrderServiceEmail).
		Msg("dupli1-order service account seeded")
	return nil
}
