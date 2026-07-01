package bootstrap

import (
	"context"
	"fmt"

	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/ports"
	"github.com/google/uuid"
)

// seedWebServiceAccount creates the dupli1-web service account when configured.
// It is idempotent: repeated calls with the same email are no-ops.
func seedWebServiceAccount(ctx context.Context, cfg Config, repo ports.UserRepository) error {
	if cfg.WebServiceEmail == "" {
		return nil
	}
	if cfg.WebServicePassword == "" {
		return fmt.Errorf("seed web service account: DUPLI1_WEB_SERVICE_PASSWORD is required when DUPLI1_WEB_SERVICE_EMAIL is set")
	}

	existing, err := repo.FindByEmail(ctx, cfg.WebServiceEmail)
	if err != nil {
		return fmt.Errorf("seed web service account: lookup: %w", err)
	}
	if existing != nil {
		return nil
	}

	u, err := domain.NewUser(
		uuid.New().String(),
		cfg.WebServiceEmail,
		cfg.WebServicePassword,
		domain.RoleCustomerRegistrar,
	)
	if err != nil {
		return fmt.Errorf("seed web service account: create: %w", err)
	}
	if err := repo.Save(ctx, u); err != nil {
		return fmt.Errorf("seed web service account: save: %w", err)
	}

	cfg.Logger.Info().
		Str("event", "web_service_account_seeded").
		Str("email", cfg.WebServiceEmail).
		Msg("dupli1-web service account seeded")
	return nil
}
